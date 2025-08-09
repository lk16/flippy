import random
from typing import Any

from flippy.arguments import Arguments
from flippy.mode.game import GameMode
from flippy.othello.board import Board
from flippy.othello.position import InvalidMove
from flippy.training.file import DEFAULT_TRAINING_FILE_PATH, TrainingFile


class TrainingMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)

        training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)
        exercises = training_file.get_exercises()
        random.shuffle(exercises)

        self.exercises = exercises
        self.exercise_index = 0
        self.node_index = 0
        self.board = self.get_new_exercise_board()

    def get_new_exercise_board(self) -> Board:
        exercise = self.exercises[self.exercise_index]

        position = (
            exercise.nodes[self.node_index]
            .position.to_position()
            .rotated(random.randint(0, 7))
        )

        return Board(position, exercise.color)

    def get_board(self) -> Board:
        return self.board

    def on_move(self, move: int) -> None:
        board = self.get_board()

        try:
            child = board.do_move(move)
        except InvalidMove:
            # TODO show wrong moves played
            print("Invalid move")  # TODO remove
            return

        exercise = self.exercises[self.exercise_index]
        node = exercise.nodes[self.node_index]

        if child.position.normalized() != node.best_move:
            print("Wrong move")  # TODO remove
            return

        if self.node_index < len(exercise.nodes) - 1:
            self.node_index += 1

            normalized_grand_child = (
                self.exercises[self.exercise_index].nodes[self.node_index].position
            )

            valid_grand_children = []

            for grand_child in child.get_children():
                if grand_child.position.normalized() == normalized_grand_child:
                    valid_grand_children.append(grand_child)

            self.board = random.choice(valid_grand_children)
            return

        self.node_index = 0

        if self.exercise_index < len(self.exercises) - 1:
            self.exercise_index += 1
        else:
            self.exercise_index = 0

        self.board = self.get_new_exercise_board()

    def get_ui_details(self) -> dict[str, Any]:
        return {}
