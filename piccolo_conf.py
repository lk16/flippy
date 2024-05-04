from piccolo.conf.apps import AppRegistry
from piccolo.engine.postgres import PostgresEngine

DB = PostgresEngine(
    config={
        "host": "localhost",
        "port": "12321",
        "user": "book_user",
        "password": "book_pass",
        "database": "flippy",
    }
)


# A list of paths to piccolo apps
# e.g. ['blog.piccolo_app']
APP_REGISTRY = AppRegistry(apps=["db_migrations.piccolo_app"])
