from __future__ import annotations

import json
import pika
import typer
from pika.adapters.blocking_connection import BlockingChannel
from pika.spec import PERSISTENT_DELIVERY_MODE, Basic, BasicProperties
from typing import Any, Dict

from flippy.book import get_learn_level
from flippy.config import (
    RABBITMQ_HOST,
    RABBITMQ_PASS,
    RABBITMQ_PORT,
    RABBITMQ_QUEUE,
    RABBITMQ_USER,
)
from flippy.db import DB, MAX_SAVABLE_DISCS
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxRequest
from flippy.othello.position import Position

app = typer.Typer(pretty_exceptions_enable=False)


class Message:
    def __init__(self, position: Position) -> None:
        self.position = position
        self.priority = position.count_empties()

    def to_json(self) -> str:
        return json.dumps(
            {
                "position": {"me": self.position.me, "opp": self.position.opp},
                "priority": self.priority,
            }
        )

    @staticmethod
    def from_json(json_str: str) -> Message:
        data: Dict[str, Any] = json.loads(json_str)
        position = Position(data["position"]["me"], data["position"]["opp"])
        return Message(position=position)


class BookWorker:
    def __init__(self) -> None:
        self.db = DB()
        self.connection = self.get_connection()
        self.channel = self.declare_channel()

    def get_connection(self) -> pika.BlockingConnection:
        credentials = pika.PlainCredentials(RABBITMQ_USER, RABBITMQ_PASS)
        params = pika.ConnectionParameters(
            host=RABBITMQ_HOST, port=RABBITMQ_PORT, credentials=credentials
        )
        return pika.BlockingConnection(params)

    def declare_channel(self) -> BlockingChannel:
        channel = self.connection.channel()
        channel.queue_declare(queue=RABBITMQ_QUEUE, arguments={"x-max-priority": 100})
        return channel

    def send_many(self, messages: list[Message]) -> None:
        for message in messages:
            self.channel.basic_publish(
                exchange="",
                routing_key=RABBITMQ_QUEUE,
                body=message.to_json(),
                properties=pika.BasicProperties(
                    delivery_mode=PERSISTENT_DELIVERY_MODE,
                    priority=message.priority,
                ),
            )

    def send(self, message: Message) -> None:
        self.send_many([message])

    def consume_loop(self) -> None:
        # TODO Disable automatic acknowledgement.
        self.channel.basic_consume(
            queue=RABBITMQ_QUEUE, on_message_callback=self.consume_callback
        )
        print("Waiting for messages")
        self.channel.start_consuming()

    def consume_callback(
        self,
        channel: BlockingChannel,
        method: Basic.Deliver,
        properties: BasicProperties,
        body: bytes,
    ) -> None:
        message = Message.from_json(body.decode())

        position = message.position
        learn_level = get_learn_level(position.count_discs())

        position.show()
        print(f"{position!r}")

        db_evaluation = self.db.lookup_edax_position(position)

        if db_evaluation.level >= learn_level:
            print("Board was already learned.")
            return

        print(f"Learning at level {learn_level}")
        print()

        request = EdaxRequest([position], learn_level, source=None)
        learned_evaluations = start_evaluation_sync(request)

        self.db.update_edax_evaluations(learned_evaluations)

    def purge(self) -> None:
        self.channel.queue_purge(queue=RABBITMQ_QUEUE)


@app.command()
def add_work() -> None:
    client = BookWorker()

    print("Purging queue")
    client.purge()

    total_position_count = 0

    for disc_count in range(4, MAX_SAVABLE_DISCS + 1):
        learn_level = get_learn_level(disc_count)

        positions = client.db.get_boards_with_disc_count_below_level(
            disc_count, learn_level
        )
        total_position_count += len(positions)

        if not positions:
            continue

        print(f"Sending positions with {disc_count:>2} discs: {len(positions)} items")

        messages = [Message(position) for position in positions]
        client.send_many(messages)

    print(f"Done. Sent {total_position_count} positions.")


@app.command()
def run() -> None:
    client = BookWorker()
    client.consume_loop()


if __name__ == "__main__":
    app()
