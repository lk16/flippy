import pytest
from datetime import date, datetime
from pathlib import Path

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game

SAMPLES_DIR = Path(__file__).parent.parent / "pgn_samples"


@pytest.fixture
def flyordie_game() -> Game:
    return Game.from_pgn(SAMPLES_DIR / "flyordie.pgn")


@pytest.fixture
def playok_normal_game() -> Game:
    return Game.from_pgn(SAMPLES_DIR / "playok_normal.pgn")


@pytest.fixture
def playok_xot_game() -> Game:
    return Game.from_pgn(SAMPLES_DIR / "playok_xot.pgn")


def test_metadata_parsing(flyordie_game: Game) -> None:
    assert flyordie_game.metadata["Event"] == "Online game"
    assert flyordie_game.metadata["Site"] == "www.flyordie.com"
    assert flyordie_game.metadata["White"] == "LK16"
    assert flyordie_game.metadata["Black"] == "ozi28"
    assert flyordie_game.metadata["Result"] == "0-1"


def test_get_date(flyordie_game: Game) -> None:
    assert flyordie_game.get_date() == date(2025, 1, 30)


def test_get_datetime(playok_normal_game: Game) -> None:
    expected = datetime(2021, 3, 11, 20, 13, 30)
    assert playok_normal_game.get_datetime() == expected


def test_get_datetime_missing(flyordie_game: Game) -> None:
    assert flyordie_game.get_datetime() is None


def test_get_players(playok_normal_game: Game) -> None:
    assert playok_normal_game.get_white_player() == "lk16"
    assert playok_normal_game.get_black_player() == "alcupone"


def test_get_winner(playok_normal_game: Game, flyordie_game: Game) -> None:
    assert playok_normal_game.get_winner() == BLACK
    assert flyordie_game.get_winner() == WHITE


def test_get_black_score(playok_normal_game: Game) -> None:
    # 47-17 result means black won by 30 points
    assert playok_normal_game.get_black_score() == 30


def test_get_color(playok_normal_game: Game) -> None:
    assert playok_normal_game.get_color("lk16") == WHITE
    assert playok_normal_game.get_color("alcupone") == BLACK
    assert playok_normal_game.get_color("unknown") is None


def test_get_color_any(playok_normal_game: Game) -> None:
    assert playok_normal_game.get_color_any(["unknown", "lk16"]) == WHITE
    assert playok_normal_game.get_color_any(["unknown"]) is None


def test_is_xot(playok_xot_game: Game, playok_normal_game: Game) -> None:
    assert playok_xot_game.is_xot() is True
    assert playok_normal_game.is_xot() is False


def test_moves_parsing(flyordie_game: Game) -> None:
    # Test first few moves from flyordie.pgn
    expected_start = [
        Board.field_to_index("e6"),
        Board.field_to_index("f4"),
        Board.field_to_index("e3"),
        Board.field_to_index("d6"),
    ]
    assert flyordie_game.moves[:4] == expected_start


def test_board_count(flyordie_game: Game) -> None:
    # Number of boards should be number of moves + 1 (starting position)
    assert len(flyordie_game.moves) + 1 == len(flyordie_game.boards)


def test_zip_board_moves(flyordie_game: Game) -> None:
    # Test that boards and moves can be zipped together
    moves_count = 0
    for board, move in flyordie_game.zip_board_moves():
        moves_count += 1
        assert board.is_valid_move(move)

    assert moves_count == len(flyordie_game.moves)
