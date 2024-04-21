import typer
from itertools import count

from flippy.othello.position import Position

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def unique_positions() -> None:
    positions: set[Position] = {Position.start()}
    print(f"{4:>2} discs | {len(positions):>9} positions")

    for discs in count(5):
        child_positions: set[Position] = set()

        for position in positions:
            for child in position.get_children():
                child_positions.add(child.normalized())

        positions = child_positions
        print(f"{discs:>2} discs | {len(positions):>9} positions")


if __name__ == "__main__":
    app()
