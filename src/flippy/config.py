import json

from pathlib import Path
from typing import cast
from flippy import PROJECT_ROOT


class Config:
    def __init__(self) -> None:
        config_path = PROJECT_ROOT / "config.json"
        self.__raw = json.loads(config_path.read_text())

    @staticmethod
    def resolve_path(string: str) -> Path:
        string = string.replace("$PROJECT_ROOT", str(PROJECT_ROOT))
        string = string.replace("~", str(Path.home()))
        return Path(string).resolve()

    def pgn_source_folders(self) -> list[Path]:
        return [
            self.resolve_path(item)
            for item in self.__raw["pgn_organizer"]["source_folders"]
        ]

    def pgn_target_folder(self) -> Path:
        return self.resolve_path(self.__raw["pgn_organizer"]["target_folder"])

    def playok_usernames(self) -> list[str]:
        return cast(list[str], self.__raw["pgn_organizer"]["playok_usernames"])

    def usernames(self) -> list[str]:
        return cast(list[str], self.__raw["usernames"])

    def all_usernames(self) -> set[str]:
        return {*self.playok_usernames(), *self.usernames()}

    def edax_path(self) -> Path:
        return self.resolve_path(self.__raw["edax_path"])


config = Config()
