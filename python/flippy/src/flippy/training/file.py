from __future__ import annotations

import json
import random
from pathlib import Path
from typing import Any

from flippy import PROJECT_ROOT
from flippy.book.api_client import APIClient
from flippy.config import PgnConfig
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.bitset import REVERSE_ROTATION
from flippy.othello.board import BLACK, WHITE, Board, opponent
from flippy.othello.game import Game
from flippy.othello.position import NormalizedPosition, Position

DEFAULT_TRAINING_FILE_PATH = PROJECT_ROOT / ".flippy/training.json"


def get_learn_level(disc_count: int) -> int:
    # TODO put this in a JSON config file so go code can load it too.
    if disc_count <= 9:
        return 44

    if disc_count <= 13:
        return 40

    if disc_count <= 16:
        return 36

    if disc_count <= 20:
        return 34

    return 32


class TrainingNode:
    def __init__(
        self,
        position: NormalizedPosition,
        rotations: set[int],
        best_move: NormalizedPosition,
        alternative_moves: set[NormalizedPosition],
    ) -> None:
        assert all(r in range(8) for r in rotations)
        assert len(rotations) > 0

        children = position.to_position().get_normalized_children()
        assert best_move in children
        assert alternative_moves.issubset(children)
        assert best_move not in alternative_moves

        self.rotations = rotations
        self.position = position
        self.best_move = best_move
        self.alternative_moves = alternative_moves

    @classmethod
    def from_json(cls, key: str, value: dict[str, Any]) -> TrainingNode:
        position = key
        rotations = set(value["rotations"])
        best_move = value["best_move"]
        alternative_moves = value["alternative_moves"]

        assert all(isinstance(r, int) for r in rotations)

        assert isinstance(position, str)
        assert isinstance(best_move, str)
        assert isinstance(alternative_moves, list)
        assert all(isinstance(move, str) for move in alternative_moves)

        return cls(
            position=NormalizedPosition.from_api(position),
            rotations=rotations,
            best_move=NormalizedPosition.from_api(best_move),
            alternative_moves=set(
                NormalizedPosition.from_api(move) for move in alternative_moves
            ),
        )

    def to_json(self) -> tuple[str, dict[str, Any]]:
        return (
            self.position.to_api(),
            {
                "rotations": sorted(self.rotations),
                "best_move": self.best_move.to_api(),
                "alternative_moves": sorted(
                    move.to_api() for move in self.alternative_moves
                ),
            },
        )


class Exercise:
    def __init__(self, color: int, nodes: list[TrainingNode]) -> None:
        self.color = color
        self.nodes = nodes


class TrainingFile:
    def __init__(self, file: Path) -> None:
        self.file = file
        self.black_nodes: dict[NormalizedPosition, TrainingNode] = {}
        self.white_nodes: dict[NormalizedPosition, TrainingNode] = {}
        self.api_client = APIClient()

        try:
            read_json = json.loads(self.file.read_text())
        except FileNotFoundError:
            print(f"Warning: Training file not found, creating new one at {self.file}")
            return

        for position_str, node in read_json["black"].items():
            position = NormalizedPosition.from_api(position_str)
            self.black_nodes[position] = TrainingNode.from_json(position_str, node)

        for position_str, node in read_json["white"].items():
            position = NormalizedPosition.from_api(position_str)
            self.white_nodes[position] = TrainingNode.from_json(position_str, node)

    def save(self) -> None:
        data: dict[str, dict[str, Any]] = {
            "black": {},
            "white": {},
        }

        for node in self.black_nodes.values():
            key, value = node.to_json()
            data["black"][key] = value

        for node in self.white_nodes.values():
            key, value = node.to_json()
            data["white"][key] = value

        self.file.write_text(json.dumps(data, indent=4, sort_keys=True))

    def add_board(self, board: Board) -> None:
        position = board.position.normalized()

        if board.turn == BLACK:
            nodes = self.black_nodes
        else:
            nodes = self.white_nodes

        if position in nodes:
            return

        children = position.to_position().get_normalized_children()
        evaluations = self.api_client.lookup_positions(children)
        child_disc_count = board.count_discs() + 1

        edax_request = EdaxRequest(
            positions=evaluations.get_missing_children(board.position),
            level=get_learn_level(child_disc_count),
            source=None,
        )

        new_evaluations: EdaxEvaluations = start_evaluation_sync(edax_request)
        self.api_client.save_learned_evaluations(new_evaluations.values())
        evaluations.update(new_evaluations)

        # Find lowest score for opponent, so best move for us.
        min_score = min(evaluation.score for evaluation in evaluations.values())

        best_evals = [
            evaluation
            for evaluation in evaluations.values()
            if evaluation.score == min_score
        ]

        if len(best_evals) == 1:
            best_move = best_evals[0].position.normalized()
        else:
            # TODO let user pick which move to use as the best_move
            best_move = random.choice(best_evals).position.normalized()

        nodes[position] = TrainingNode(
            position=position,
            rotations=self._get_normalized_position_rotations(board.turn, position),
            best_move=best_move,
            alternative_moves=set(),
        )

    def _get_normalized_position_rotations(
        self, color: int, position: NormalizedPosition
    ) -> set[int]:
        start_normalized = Position.start().normalized()
        first_move_normalized = Position.start().do_move(19).normalized()

        if position == start_normalized:
            return {0}

        if position == first_move_normalized:
            return {
                REVERSE_ROTATION[child.normalize()[1]]
                for child in start_normalized.to_position().get_children()
            }

        if color == BLACK:
            nodes = self.black_nodes
        else:
            nodes = self.white_nodes

        potential_grand_parents: list[Board] = []
        grand_parent_disc_count = position.to_position().count_discs() - 2

        for node in nodes.values():
            if node.position.to_position().count_discs() == grand_parent_disc_count:
                for rotation in node.rotations:
                    grand_parent_position = node.position.to_position().rotated(
                        rotation
                    )
                    potential_grand_parents.append(Board(grand_parent_position, color))

        rotations: set[int] = set()

        for potential_grand_parent in potential_grand_parents:
            for potential_parent in potential_grand_parent.get_children():
                for potential_rotated in potential_parent.get_children():
                    if potential_rotated.position.normalized() == position:
                        rotation = potential_rotated.position.normalize()[1]
                        rotations.add(REVERSE_ROTATION[rotation])

        if not rotations:
            raise ValueError(f"No rotations found for {position} with color {color}.")

        return rotations

    def add_game(self, game: Game) -> None:
        all_usernames = PgnConfig().all_usernames

        color = game.get_color_any(all_usernames)

        if color is None:
            print(f"Warning: Neither players are in config: {game.file}")
            return

        # Add all correct boards where we played the best move plus the first where we didn't.
        for board, next_board in zip(game.boards, game.boards[1:]):
            # Skip boards where we didn't play.
            if board.turn != color:
                continue

            self.add_board(board)

            if not self.is_best_move(board, next_board):
                break

    def is_best_move(self, board: Board, next_board: Board) -> bool:
        position = board.position.normalized()
        next_position = next_board.position.normalized()

        children = position.to_position().get_normalized_children()
        evaluations = self.api_client.lookup_positions(children)
        child_disc_count = board.count_discs() + 1

        edax_request = EdaxRequest(
            positions=evaluations.get_missing_children(board.position),
            level=get_learn_level(child_disc_count),
            source=None,
        )

        new_evaluations: EdaxEvaluations = start_evaluation_sync(edax_request)
        self.api_client.save_learned_evaluations(new_evaluations.values())
        evaluations.update(new_evaluations)

        # Find lowest score for opponent, so best move for us.
        min_score = min(evaluation.score for evaluation in evaluations.values())

        return evaluations[next_position].score == min_score

    def print_ascii_art(self) -> None:
        for color, nodes in [(BLACK, self.black_nodes), (WHITE, self.white_nodes)]:
            for node in nodes.values():
                print(f"Node normalized: {node.position.to_api()}")

                print("Board:")
                board = Board(node.position.to_position(), color)
                board.show()

                print("Best move:")
                children = board.get_children()

                # Best move is normalized, so we need to rotate the board it to show it.
                for rot in range(8):
                    child_position = node.best_move.to_position().rotated(rot)
                    child_board = Board(child_position, opponent(color))

                    if child_board in children:
                        child_board.show()
                        break

                if node.alternative_moves:
                    print(f"Found {len(node.alternative_moves)} alternative moves.")

                print()

        print("---")
        print()

        print(f"Black nodes: {len(self.black_nodes)}")
        print(f"White nodes: {len(self.white_nodes)}")

        total_nodes = len(self.black_nodes) + len(self.white_nodes)
        print("Total nodes:", total_nodes)

    def get_exercises(self) -> list[Exercise]:
        black_root = Position.start().normalized()

        # TODO it seems white_root is not found
        white_root = Position.start().do_move(19).normalized()

        def _get_exercises(color: int, node: TrainingNode) -> list[Exercise]:
            if color == BLACK:
                nodes = self.black_nodes
            else:
                nodes = self.white_nodes

            children = node.position.to_position().get_normalized_children()

            grand_children: set[NormalizedPosition] = set()
            for child in children:
                grand_children.update(child.to_position().get_normalized_children())

            missing_grand_children = grand_children - set(nodes.keys())

            # All grandchildren are missing
            if len(missing_grand_children) == len(grand_children):
                return [Exercise(color, [node])]

            exercises: list[Exercise] = []
            for grand_child in grand_children:
                if grand_child in nodes:
                    grand_child_exercises = _get_exercises(color, nodes[grand_child])
                    for grand_child_exercise in grand_child_exercises:
                        grand_child_exercise.nodes.insert(0, node)

                    exercises.extend(grand_child_exercises)

            return exercises

        exercises: list[Exercise] = []

        if black_root in self.black_nodes:
            exercises += _get_exercises(BLACK, self.black_nodes[black_root])

        if white_root in self.white_nodes:
            exercises += _get_exercises(WHITE, self.white_nodes[white_root])

        return exercises

    def list_exercises(self) -> None:
        exercises = self.get_exercises()
        for exercise in exercises:
            for node in exercise.nodes:
                print(f"{node.position.to_api()} -> {node.best_move.to_api()}")

            print()

        print(f"Total exercises: {len(exercises)}")
