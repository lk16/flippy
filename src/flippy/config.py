import os
from dotenv import load_dotenv
from pathlib import Path

from flippy import PROJECT_ROOT

load_dotenv()


def resolve_path(string: str) -> Path:
    string = string.replace("PROJECT_ROOT", str(PROJECT_ROOT))
    string = string.replace("~", str(Path.home()))
    return Path(string).resolve()


def resolve_paths(string: str) -> list[Path]:
    return [resolve_path(part) for part in string.split(",")]


def get_db_dsn() -> str:
    user = os.environ["FLIPPY_POSTGRES_USER"]
    pass_ = os.environ["FLIPPY_POSTGRES_PASS"]
    host = os.environ["FLIPPY_POSTGRES_HOST"]
    port = int(os.environ["FLIPPY_POSTGRES_PORT"])
    db = os.environ["FLIPPY_POSTGRES_DB"]

    return f"postgres://{user}:{pass_}@{host}:{port}/{db}"


class PgnConfig:
    def __init__(self) -> None:
        self.source_folders = resolve_paths(os.environ["FLIPPY_PGN_SOURCE_FOLDERS"])
        self.target_folder = resolve_path(os.environ["FLIPPY_PGN_TARGET_FOLDER"])

        self.playok_usernames = os.environ["FLIPPY_PLAYOK_USERNAMES"].split(",")
        self.usernames = os.environ["FLIPPY_USERNAMES"].split(",")

        self.all_usernames = {*self.playok_usernames, *self.usernames}


def get_edax_path() -> Path:
    return resolve_path(os.environ["FLIPPY_EDAX_PATH"])


def get_edax_verbose() -> bool:
    return os.getenv("FLIPPY_EDAX_VERBOSE", "0") != "0"


def get_book_server_url() -> str:
    return os.environ["FLIPPY_BOOK_SERVER_URL"]


class BookServerConfig:
    def __init__(self) -> None:
        self.host = os.environ["FLIPPY_BOOK_SERVER_HOST"]
        self.port = int(os.environ["FLIPPY_BOOK_SERVER_PORT"])
        self.basic_auth_user = os.environ["FLIPPY_BOOK_SERVER_BASIC_AUTH_USER"]
        self.basic_auth_pass = os.environ["FLIPPY_BOOK_SERVER_BASIC_AUTH_PASS"]


def get_book_server_token() -> str:
    return os.environ["FLIPPY_BOOK_SERVER_TOKEN"]
