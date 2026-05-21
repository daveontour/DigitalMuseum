To start everything:

Bash
docker compose up -d
To stop and remove the container:

Bash
docker compose down
To rebuild after you change your Go code:

Bash
docker compose up -d --build

Without Compose 

Build & Run Instructions
1. Build the image
From the project root:


docker build -t digitalmuseum:latest .
2. Run the container
The app reads all configuration from environment variables (no .env file inside the image). Pass them with -e or --env-file:


docker run -d \
  --name digitalmuseum \
  -p 8080:8080 \
  --env-file .env \
  digitalmuseum:latest