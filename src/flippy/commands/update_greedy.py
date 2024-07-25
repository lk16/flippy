import datetime
import typer
from typing import Optional

from flippy.book import get_learn_level
from flippy.db import DB
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations, EdaxRequest
from flippy.othello.position import Position

GREEDY_MIN_LEARN_LEVEL = 26


class GreedyEvaluation:
    def __init__(self, score: int, best_move: int, saved: bool = False):
        self.score = score
        self.best_move = best_move
        self.saved = saved


class GreedyUpdater:
    def __init__(self) -> None:
        self.db = DB()
        self.edax = EdaxEvaluations()
        self.greedy: dict[Position, GreedyEvaluation] = {}

    def __call__(self) -> None:
        print("Looking up positions in DB")

        # Load entire DB into memory
        self.edax = self.db.get_edax_evaluations_sorted_by_disc_count()

        self.greedy = {
            position: GreedyEvaluation(score, best_move, saved=True)
            for position, score, best_move in self.db.get_greedy_evaluations()
        }

        for disc_count in range(4, 64):
            positions = [
                position
                for position in self.edax.values
                if position.count_discs() == disc_count
            ]

            total_seconds = 0.0

            for i, position in enumerate(positions):
                before = datetime.datetime.now()
                self.learn_greedy(position)
                # TODO update preceding positions
                self.save_learned_greedy()

                after = datetime.datetime.now()
                seconds = (after - before).total_seconds()

                total_seconds += seconds
                average = total_seconds / (i + 1)

                eta = datetime.datetime.now() + datetime.timedelta(
                    seconds=average * (len(positions) - (i + 1))
                )

                print(
                    f"greedy learning | {i+1}/{len(positions)} | {disc_count:>2} discs | {seconds:7.3f} sec "
                    + f"| ETA {eta.strftime('%Y-%m-%d %H:%M:%S')}"
                )

    def find_evaluation(self, position: Position) -> Optional[EdaxEvaluation]:
        try:
            edax_evaluation = self.edax.lookup(position)
        except KeyError:
            return None  # Not found

        if edax_evaluation.level >= get_learn_level(position.count_discs()):
            return edax_evaluation

        return None  # Evaluation not reliable enough

    def compute_evaluation(self, position: Position) -> EdaxEvaluation:
        level = get_learn_level(position.count_discs())
        request = EdaxRequest([position], level=level, source=None)

        print(f"Computing position with discs={position.count_discs()} level={level}")

        response = start_evaluation_sync(request)

        self.edax.update(response)

        # Lookup should not fail
        evaluation = response.lookup(position)

        self.db.save_edax(evaluation)

        return evaluation

    def learn_greedy(self, position: Position) -> int:
        evaluation = self.find_evaluation(position)

        if not evaluation:
            evaluation = self.compute_evaluation(position)

        best_move = evaluation.best_moves[0]

        # Compute until first estimate of exact score
        if evaluation.depth + position.count_discs() == 64:
            self.greedy[position] = GreedyEvaluation(evaluation.score, best_move)
            return evaluation.score

        best_child = position.do_normalized_move(best_move)
        child_score = self.learn_greedy(best_child)

        score = -child_score

        self.greedy[position] = GreedyEvaluation(score, best_move)
        return score

    def save_learned_greedy(self) -> None:
        for position, greedy_eval in self.greedy.items():
            if not greedy_eval.saved:
                self.db.save_greedy(position, greedy_eval.score, greedy_eval.best_move)
                greedy_eval.saved = True


app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def greedy_updater() -> None:
    GreedyUpdater()()


if __name__ == "__main__":
    app()
