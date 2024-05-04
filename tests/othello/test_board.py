import pytest
from typing import Iterable

from flippy.othello.board import Board
from flippy.othello.position import PASS_MOVE


@pytest.mark.parametrize(
    ["index", "expected"],
    [
        pytest.param(0, "a1", id="index-0"),
        pytest.param(7, "h1", id="index-7"),
        pytest.param(56, "a8", id="index-56"),
        pytest.param(63, "h8", id="index-63"),
        pytest.param(17, "b3", id="index-17"),
    ],
)
def test_index_to_field_ok(index: int, expected: str) -> None:
    assert Board.index_to_field(index) == expected


@pytest.mark.parametrize(
    ["index"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_index_to_field_error(index: int) -> None:
    with pytest.raises(ValueError):
        Board.index_to_field(index)


@pytest.mark.parametrize(
    ["indexes", "expected"],
    [
        pytest.param([], "", id="0-indexes"),
        pytest.param([0], "a1", id="1-index"),
        pytest.param([0, 1], "a1 b1", id="2-indexes"),
    ],
)
def test_indexes_to_fields(indexes: Iterable[int], expected: str) -> None:
    assert Board.indexes_to_fields(indexes) == expected


@pytest.mark.parametrize(
    ["field", "expected"],
    [
        pytest.param("a1", 0, id="field-a1"),
        pytest.param("h1", 7, id="field-h1"),
        pytest.param("a8", 56, id="field-a8"),
        pytest.param("h8", 63, id="field-h8"),
        pytest.param("b3", 17, id="field-b3"),
        pytest.param("A1", 0, id="field-A1"),
        pytest.param("H1", 7, id="field-H1"),
        pytest.param("A8", 56, id="field-A8"),
        pytest.param("H8", 63, id="field-H8"),
        pytest.param("B3", 17, id="field-B3"),
        pytest.param("--", PASS_MOVE, id="field---"),
        pytest.param("ps", PASS_MOVE, id="field-ps"),
        pytest.param("PS", PASS_MOVE, id="field-PS"),
    ],
)
def test_field_to_index_ok(field: str, expected: int) -> None:
    assert Board.field_to_index(field) == expected


@pytest.mark.parametrize(
    ["field"],
    [
        pytest.param("", id="empty"),
        pytest.param("a", id="to-short"),
        pytest.param("aaa", id="too-long"),
        pytest.param("a9", id="invalid-row"),
        pytest.param("i8", id="invalid-column"),
    ],
)
def test_field_to_index_error(field: str) -> None:
    with pytest.raises(ValueError):
        assert Board.field_to_index(field)


# TODO add other tests for Board
