# lidoo

Create the shared database configuration before starting the services:

```sh
cp .env.example .env
docker compose up -d db
go run ./cmd up --name testing --version 18
```
