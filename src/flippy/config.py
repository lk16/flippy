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


PGN_SOURCE_FOLDERS = resolve_paths(os.environ["FLIPPY_PGN_SOURCE_FOLDERS"])
PGN_TARGET_FOLDER = resolve_path(os.environ["FLIPPY_PGN_TARGET_FOLDER"])

PLAYOK_USERNAMES = os.environ["FLIPPY_PLAYOK_USERNAMES"].split(",")
USERNAMES = os.environ["FLIPPY_USERNAMES"].split(",")

ALL_USERNAMES = {*PLAYOK_USERNAMES, *USERNAMES}

EDAX_PATH = resolve_path(os.environ["FLIPPY_EDAX_PATH"])

POSTGRES_USER = os.environ["FLIPPY_POSTGRES_USER"]
POSTGRES_PASS = os.environ["FLIPPY_POSTGRES_PASS"]
POSTGRES_HOST = os.environ["FLIPPY_POSTGRES_HOST"]
POSTGRES_PORT = int(os.environ["FLIPPY_POSTGRES_PORT"])
POSTGRES_DB = os.environ["FLIPPY_POSTGRES_DB"]

POSTGRES_DSN = f"postgres://{POSTGRES_USER}:{POSTGRES_PASS}@{POSTGRES_HOST}:{POSTGRES_PORT}/{POSTGRES_DB}"

BOOK_LEARNING_SERVER_URL = os.environ["FLIPPY_BOOK_LEARNING_SERVER_URL"]
