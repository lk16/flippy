from multiprocessing import Queue
from pathlib import Path

from flippy.edax.evaluations import EdaxEvaluations
from flippy.edax.process import EdaxProcess
from flippy.othello.board import BLACK, PASS_MOVE, WHITE, Board
from flippy.othello.game import Game


class PgnAnanlyzer:
    def __init__(self, file: Path, level: int) -> None:
        self.file = file
        self.level = level
        self.game = Game.from_pgn(file)
        self.evaluations = EdaxEvaluations({})

    def _get_best(self, board: Board) -> tuple[list[int], int]:
        child_scores: list[tuple[int, int]] = []

        for move in board.get_moves_as_set():
            child = board.do_move(move)
            score = self.evaluations.lookup(child).score
            child_scores.append((move, score))

        # Take minimum, because scores are from opponent's point of view
        best_score = min(score for _, score in child_scores)
        best_moves = [move for move, score in child_scores if score == best_score]

        return best_moves, best_score

    def __call__(self) -> None:
        all_children = self.game.get_all_children()
        edax_proc = EdaxProcess(all_children, self.level, Queue(), None)
        self.evaluations = edax_proc.search_sync()

        for move_offset, (board, played_move) in enumerate(self.game.zip_board_moves()):
            turn = {WHITE: "●", BLACK: "○"}[board.turn]
            possible_moves = board.get_moves_as_set()

            if not possible_moves:
                assert played_move == PASS_MOVE
                print(f"{move_offset+1:>2}. {turn} --")
                continue

            best_moves, best_score = self._get_best(board)
            best_score_str = self.get_colored_score(best_score, board)
            best_fields = ",".join(Board.index_to_field(move) for move in best_moves)

            played_field = Board.index_to_field(played_move)
            played_child = board.do_move(played_move)
            score = self.evaluations.lookup(played_child).score
            score_str = self.get_colored_score(score, board)

            output_line = f"{move_offset+1:>2}. {turn} {played_field} {score_str}"

            if played_move not in best_moves:
                output_line += f" (best: {best_score_str} @ {best_fields})"

            print(output_line)

    def get_colored_score(self, score: int, board: Board) -> str:
        if score == 0:
            return "◑ + 0"

        if (score > 0) != (board.turn == BLACK):
            return f"○ +{abs(score):>2}"

        return f"● +{abs(score):>2}"
