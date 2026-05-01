# Page Rank

The Page Rank service is responsible for calculating the PageRank of web pages based on the backlinks data stored in MongoDB. It uses Google's original PageRank algorithm to determine the importance of each page. The PageRank data is stored in MongoDB for fast retrieval by other services.

## Setup

### Using Docker
Using Docker is the recommended way to run the Page Rank service. It allows you to run the service in an isolated environment without worrying about dependencies or system configurations. To properly run the service with Docker you need to add a `variables.env` file in the `services/page-rank` directory. The `variables.env` file should contain the following variables:

```bash
MONGO_HOST=<your_mongo_host>
MONGO_DB=<your_mongo_db>             # default: test
MONGO_USERNAME=<your_mongo_username> # default: empty
MONGO_PASSWORD=<your_mongo_password> # default: empty
```

To run the Page Rank service using Docker, follow these steps:
1. **Install Docker**: The installation instructions will depend on your operating system. You can find the installation instructions for your OS on the [Docker website](https://docs.docker.com/get-docker/).
2. **Build the Docker image**: Navigate to the `services/page-rank` directory (if you're not already here) and run the following command.
   ```bash
   docker compose up --build
   ```
3. **Running in detached mode**: If you want to run the Page Rank service in the background, you can use the `-d` flag.
   ```bash
   docker compose up --build -d
   ```
4. **Scaling the Page Rank service**: If you want to run multiple instances of the Page Rank service when it is running under detached mode, you can use the `--scale` option.
   ```bash
   docker compose up --scale page_rank=3
   ```
5. **Stopping the Page Rank service**: To stop the Page Rank service, you can use the following command.
   ```bash
   # If you are running in detached mode
   docker compose down
   # If you are running in the foreground
   Ctrl + C
   ```

### Without Docker

If you prefer to run the Page Rank service without Docker, you can do so by building and running the Go binary directly on your system. Make sure you have Go installed (version 1.18 or higher is recommended).

1. **Install Go**:  
   Download and install Go from the [official website](https://go.dev/dl/).

2. **Set up environment variables**:  
   Create a `variables.env` file in the `services/page-rank` directory with the following content (adjust values as needed):
   ```bash
   MONGO_HOST=<your_mongo_host>
   MONGO_DB=<your_mongo_db>             # default: test
   MONGO_USERNAME=<your_mongo_username> # default: empty
   MONGO_PASSWORD=<your_mongo_password> # default: empty
   ```

3. **Export environment variables**:  
   Before running the Page Rank service, export the variables in your shell:
   ```bash
   export $(grep -v '^#' variables.env | xargs)
   ```

4. **Build the Page Rank service**:  
   Navigate to the `services/page-rank` directory and run:
   ```bash
   go build -o page_rank ./cmd/page_rank
   ```

5. **Run the Page Rank service**:  
   Start the Page Rank service with optional flags for concurrency and batch size:
   ```bash
   ./page_rank -max-concurrency=10 -max-pages=100
   ```

6. **Stopping the Page Rank service**:  
   Press `Ctrl + C` in the terminal to stop the process.

**Note:**  
- Make sure MongoDB is running and accessible with the credentials you provided.
- You may need to install Go dependencies using `go mod tidy` before building.

For development or debugging, you can also run the Page Rank service directly:
```bash
go run
```
