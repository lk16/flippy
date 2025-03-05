from flippy.mode.training.exercise import Exercise
from flippy.mode.training.exercise_list import (
    BLACK_TREE,
    WHITE_TREE,
    Node,
    get_exercises,
)
from flippy.othello.board import Board
from flippy.othello.position import InvalidMove, NormalizedPosition


def test_exercise_validity() -> None:
    # Validation happens inside constructors in get_exercises().
    # This should not fail.
    _ = get_exercises()


def test_exercise_order() -> None:
    def sort_key(exercise: Exercise) -> tuple[int, str]:
        return (exercise.color, exercise.raw)

    exercises = get_exercises()

    assert exercises == sorted(
        exercises, key=sort_key
    ), "Exercises should be ordered first by color, then by raw"


def check_move_count(prefix: str, tree: Node) -> None:
    moves = tree.moves.split()

    if len(moves) > 2 or len(moves) == 0:
        raise AssertionError(f"At {prefix}: Invalid number of moves: {len(moves)}")


def apply_moves(board: Board, moves: list[str], prefix: str = "") -> Board:
    """Apply a sequence of moves to a board, raising AssertionError if any move is invalid."""
    current_board = board
    move_path = prefix

    for i, move in enumerate(moves):
        try:
            current_board = current_board.do_move(Board.field_to_index(move))
        except InvalidMove:
            move_context = f"({' '.join(moves[:i])})" if i > 0 else ""
            raise AssertionError(
                f"At {move_path} {move_context}: Invalid move '{move}'"
            )

    return current_board


def check_move_validity(prefix: str, tree: Node, board: Board) -> Board:
    moves = tree.moves.split()
    return apply_moves(board, moves, prefix)


def check_child_moves_set(prefix: str, tree: Node, board: Board) -> None:
    moves = tree.moves.split()

    if tree.children and len(moves) != 2:
        raise AssertionError(f"At {prefix}: Non-leaf node should have two moves")

    if not tree.children:
        return

    # Apply the two moves from the current node
    grand_child_board = apply_moves(board, moves, prefix)

    # Track first moves to ensure uniqueness and completeness
    valid_moves = {
        Board.index_to_field(i) for i in grand_child_board.get_moves_as_set()
    }
    seen_first_moves = set()

    # Check each child node
    for child in tree.children:
        first_child_move = child.moves.split()[0]

        try:
            Board.field_to_index(first_child_move)
        except ValueError:
            raise AssertionError(
                f"At {prefix}: Invalid first move '{first_child_move}'"
            )

        if first_child_move in seen_first_moves:
            raise AssertionError(
                f"At {prefix}: Duplicate first move '{first_child_move}'"
            )

        seen_first_moves.add(first_child_move)

    # Check if all valid moves are covered
    missing_moves = valid_moves - seen_first_moves
    if missing_moves:
        raise AssertionError(
            f"At {prefix}: Missing valid first moves: {sorted(missing_moves)}"
        )


def check_tree_integrity(prefix: str, node: Node, board: Board) -> None:
    assert board.count_discs() == 4 + len(prefix.split())

    check_move_count(prefix, node)
    check_child_moves_set(prefix, node, board)

    child_board = check_move_validity(prefix, node, board)

    if node.transposition is not None:
        if node.children:
            raise AssertionError(
                f"At {prefix}: Transposition move should not have children"
            )

        if node.eval is not None:
            raise AssertionError(f"At {prefix}: Transposition should not have eval")

    # Recursively check subtree
    next_prefix = f"{prefix} {node.moves}"

    for child in node.children:
        check_tree_integrity(next_prefix, child, child_board)


def test_black_tree_integrity() -> None:
    assert len(BLACK_TREE.moves.split()) == 1, "Black tree root should have one move"

    board = Board.start().do_move(Board.field_to_index(BLACK_TREE.moves))

    for child in BLACK_TREE.children:
        check_tree_integrity(BLACK_TREE.moves, child, board)


def test_white_tree_integrity() -> None:
    assert WHITE_TREE.moves == "", "White tree root should have no moves"

    board = Board.start()

    for child in WHITE_TREE.children:
        check_tree_integrity("", child, board)


def check_transpositions(tree: Node) -> None:
    transpositions: dict[NormalizedPosition, str] = {}

    def check_transpositions_recursive(prefix: str, tree: Node, board: Board) -> None:
        child_board = apply_moves(board, tree.moves.split(), prefix)

        next_prefix = f"{prefix} {tree.moves}".strip()

        normalized = child_board.position.normalized()

        try:
            found = transpositions[normalized]
        except KeyError:
            if tree.transposition is not None:
                raise AssertionError(f"At {next_prefix}: Transposition not found")

            transpositions[normalized] = next_prefix
        else:
            if tree.transposition != found:
                raise AssertionError(
                    f"At {next_prefix}: Invalid/missing transposition: {found}"
                )

        for child in tree.children:
            check_transpositions_recursive(next_prefix, child, child_board)

    check_transpositions_recursive("", tree, Board.start())


def test_black_tree_transpositions() -> None:
    check_transpositions(BLACK_TREE)


def test_white_tree_transpositions() -> None:
    check_transpositions(WHITE_TREE)
