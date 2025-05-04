import json
import os
import pytz
import requests
import time
import typer
import websocket
from datetime import datetime
from itertools import count
from pathlib import Path
from typing import Any, Dict, Optional

from flippy.config import PgnConfig
from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game
from flippy.othello.position import PASS_MOVE

MSG_CONNECTED_PLAYERS = "f19683a8"
MSG_ANNOUNCE_CLIENT = "efa2bd1b"
MSG_ANNOUNCE_ACCOUNT = "b9345c05"
MSG_GAME_TRANSCRIPT = "ba276087"


class OthelloQuestDownloader:
    def __init__(self) -> None:
        self.ws: websocket.WebSocket | None = None
        self.message_counter = 0
        self.verbose = "DEBUG_OTHELLO_QUEST" in os.environ

    def get_websocket_suffix(self) -> str:
        timestamp = int(time.time() * 1000)
        response = requests.get(
            f"http://questgames.net:3002/socket.io/1/?t={timestamp}"
        )
        response.raise_for_status()
        return response.text.split(":")[0]

    def connect(self) -> None:
        """Connect to the Othello Quest WebSocket server."""
        suffix = self.get_websocket_suffix()
        ws_url = f"ws://questgames.net:3002/socket.io/1/websocket/{suffix}"

        self.ws = websocket.create_connection(ws_url)

    def disconnect(self) -> None:
        """Disconnect from the Othello Quest WebSocket server."""
        if self.ws:
            self.ws.close()
            self.ws = None

    def send_message(self, number: int, data: Dict[str, Any] | None) -> None:
        """Send a message to the Othello Quest server."""
        if not self.ws:
            raise RuntimeError("Not connected to server")

        if data is None:
            message = f"{number}::"
        else:
            message = f"{number}:::{json.dumps(data)}"

        if self.verbose:
            print(f"> {message}")

        self.ws.send(message)
        self.message_counter += 1

    def receive_message(self) -> tuple[int, Dict[str, Any] | None]:
        """Receive a message from the Othello Quest server."""
        if not self.ws:
            raise RuntimeError("Not connected to server")

        response = str(self.ws.recv())

        if ":::" in response:
            number_char, msg = response.split(":::", 1)
        elif "::" in response:
            number_char, msg = response.split("::", 1)
        else:
            print(f"Received unexpected message: {response}")
            exit(1)

        number = int(number_char)

        if msg == "":
            parsed_msg = None
        else:
            try:
                parsed_msg = json.loads(msg)
            except json.JSONDecodeError:
                print(f"Received invalid JSON: {msg}")
                exit(1)

        print_response = True

        if number == 5:
            assert parsed_msg is not None

            # Ignore update on number of connected players
            if parsed_msg["name"] == MSG_CONNECTED_PLAYERS:
                print_response = False

        if self.verbose and print_response:
            print(f"< {response}")

        return number, parsed_msg

    def wait_for_message_without_type(self, number: int) -> None:
        while True:
            recv_number, recv_msg = self.receive_message()

            if recv_number == number and recv_msg is None:
                return recv_msg

    def wait_for_message_with_type(
        self, number: int, message_type: Optional[str]
    ) -> Dict[str, Any]:
        while True:
            recv_number, recv_msg = self.receive_message()

            if (
                recv_number == number
                and recv_msg is not None
                and recv_msg["name"] == message_type
            ):
                return recv_msg

    def generate_pgn_file(self, data: Dict[str, Any]) -> str:
        """Generate a PGN string from the game data."""
        game_data = data["args"][0]
        players = game_data["players"]
        moves = game_data["position"]["moves"]

        # First player is black, second is white
        black_player = players[0]
        white_player = players[1]

        # Parse and format the date and time using datetime
        utc_datetime = datetime.fromisoformat(
            game_data["created"].replace("Z", "+00:00")
        )
        utc_timezone = pytz.UTC
        copenhagen_timezone = pytz.timezone("Europe/Copenhagen")

        # Convert to Copenhagen time for filename
        copenhagen_datetime = utc_datetime.replace(tzinfo=utc_timezone).astimezone(
            copenhagen_timezone
        )
        formatted_date = copenhagen_datetime.strftime("%Y.%m.%d")
        formatted_time = copenhagen_datetime.strftime("%H:%M:%S")

        board = Board.start()

        for entry in moves:
            try:
                entry = entry["m"]
            except KeyError:
                # If a player resigns, there is no move
                continue

            if entry == "-":
                field = PASS_MOVE
            else:
                field = Board.field_to_index(entry)

            board = board.do_move(field)

        black_discs = board.count(BLACK)
        white_discs = board.count(WHITE)

        # Create the PGN header
        pgn = [
            '[Event "Online game"]',
            '[Site "questgames.net"]',
            f'[Date "{formatted_date}"]',
            f'[Time "{formatted_time}"]',
            '[Round "-"]',
            f'[White "{white_player["name"]}"]',
            f'[Black "{black_player["name"]}"]',
            f'[Result "{black_discs}-{white_discs}"]',
            f'[WhiteRating "{white_player["oldR"]:.0f}"]',
            f'[BlackRating "{black_player["oldR"]:.0f}"]',
            '[Termination "normal"]',
            f'[TimeControl "{game_data["tcb"] / 1000:.0f}"]',
            f'[UTCDate "{formatted_date}"]',
            "",
        ]

        # Add the moves
        move_line = []

        for i, entry in enumerate(moves):
            move_number = (i // 2) + 1
            if i % 2 == 0:  # Black's move
                move_line.append(f"{move_number}.")

            try:
                move = entry["m"]
            except KeyError:
                # If a player resigns, there is no move
                continue

            if move == "-":
                move = "--"

            move_line.append(move)

        # Combine everything
        pgn.append(" ".join(move_line))
        pgn_string = "\n".join(pgn)
        return pgn_string

    def run(self) -> str:
        self.connect()
        self.wait_for_message_without_type(1)

        if self.verbose:
            print("Got message indicating connection is established.")

        # ---

        message = {
            "name": MSG_ANNOUNCE_CLIENT,
            "args": [{"env": "WEB", "handicapV": "1", "gtype": "reversi"}],
        }

        self.send_message(5, message)
        self.wait_for_message_with_type(5, MSG_CONNECTED_PLAYERS)

        if self.verbose:
            print("Announced client successfully.")

        # ---

        message = {
            "name": MSG_ANNOUNCE_ACCOUNT,
            "args": [{"gtype": "reversi", "pass": "3p2enzmwh6"}],
        }

        self.send_message(5, message)
        msg = self.wait_for_message_with_type(5, MSG_ANNOUNCE_ACCOUNT)

        if self.verbose:
            print("Announced account successfully.")

        token = msg["args"][0]["token"]
        last_game = msg["args"][0]["lastGame"]

        # ---

        message = {
            "name": MSG_GAME_TRANSCRIPT,
            "args": [{"id": last_game, "user_id": "lk16_", "token": token}],
        }

        self.send_message(5, message)
        msg = self.wait_for_message_with_type(5, MSG_GAME_TRANSCRIPT)

        if self.verbose:
            print("Got game transcript successfully.")

        self.disconnect()

        # ---

        return self.generate_pgn_file(msg)


class PgnOrganizer:
    def __init__(self) -> None:
        self.pgn_config = PgnConfig()
        self.source_folders = self.pgn_config.source_folders
        self.target_folder = self.pgn_config.target_folder
        self.playok_usernames = self.pgn_config.playok_usernames

    def __call__(self) -> None:
        moved = self.move_from_source_folders()

        if moved != 0:
            print(f"Moved {moved} PGN files to {self.target_folder}")

        newly_downloaded = self.download_from_playok()

        if newly_downloaded != 0:
            print(f"Downloaded {newly_downloaded} PGN files from playok")

        newly_downloaded = self.download_from_othello_quest()

        if newly_downloaded != 0:
            print(f"Downloaded {newly_downloaded} PGN files from othello quest")

    def move_from_source_folders(self) -> int:
        pgn_files: list[tuple[Path, datetime]] = []

        for source_folder in self.source_folders:
            for pgn_file in source_folder.rglob("*.pgn"):
                created_unix = os.stat(pgn_file).st_ctime
                created = datetime.fromtimestamp(created_unix)
                pgn_files.append((pgn_file, created))

        for pgn_file, created in sorted(pgn_files, key=lambda x: x[1]):
            game = Game.from_pgn(pgn_file)
            target_path = self.get_organized_path(game, created)
            # print(f"{pgn_file} -> {target_path}")
            target_path.parent.mkdir(parents=True, exist_ok=True)
            pgn_file.rename(target_path)

        return len(pgn_files)

    def get_organized_path(self, game: Game, created: datetime) -> Path:
        game_timestamp = game.get_datetime()
        target_path = self.target_folder

        if game.is_xot():
            target_path /= "xot"
        else:
            target_path /= "normal"

        if game_timestamp:
            return target_path / game_timestamp.strftime("%Y/%m/%d/%H_%M_%S.pgn")

        game_date = game.get_date()

        if created.date() == game_date:
            return target_path / created.strftime("%Y/%m/%d/%H_%M_%S.pgn")

        target_path /= game_date.strftime("%Y/%m/%d")

        for i in count(1):
            potential_path = target_path / f"{i:04}.pgn"

            if not potential_path.exists():
                return potential_path

        raise NotImplementedError  # Unreachable

    def download_from_playok(self) -> int:
        raw_pgns: list[str] = []
        raw_pgn_lines: list[str] = []

        for username in self.playok_usernames:
            response = requests.get(f"https://www.playok.com/p/?uid={username}&g=rv")
            response.raise_for_status()

            lines = response.text.split("\n")

            for line_offset, line in enumerate(lines):
                if line_offset == 0:
                    prev_line_empty = False
                else:
                    prev_line_empty = lines[line_offset - 1].strip() == ""

                # We detect a new PGN by an empty line followed by metadata
                if prev_line_empty and line.startswith("["):
                    raw_pgn = "\n".join(raw_pgn_lines)
                    raw_pgns.append(raw_pgn)
                    raw_pgn_lines = []

                raw_pgn_lines.append(line.strip())

        # Add last PGN of file
        raw_pgn = "\n".join(raw_pgn_lines)
        raw_pgns.append(raw_pgn)

        new_files = 0

        # PGN's come in chronolgical order.
        # Using a reversed loop, we can break out of loop early if we find an existing file,
        # which would be the latest file we got in an earlier run.
        for raw_pgn in reversed(raw_pgns):
            game = Game.from_string(raw_pgn)
            file = self.get_organized_path(game, datetime.now())
            if file.exists():
                break
            new_files += 1
            file.parent.mkdir(exist_ok=True, parents=True)
            file.write_text(raw_pgn)

        return new_files

    def download_from_othello_quest(self) -> int:
        downloader = OthelloQuestDownloader()
        pgn_string = downloader.run()
        game = Game.from_string(pgn_string)
        file = self.get_organized_path(game, datetime.now())
        file.parent.mkdir(exist_ok=True, parents=True)

        if file.exists():
            return 0

        file.write_text(pgn_string)
        return 1


app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def pgn_organizer() -> None:
    PgnOrganizer()()


if __name__ == "__main__":
    app()
