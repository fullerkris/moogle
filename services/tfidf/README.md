# TF-IDF Processor

The TF-IDF Processor is a core service in the Moogle search engine pipeline. Its job is to compute TF-IDF (Term Frequency-Inverse Document Frequency) after the Indexer has processed the crawled web pages. The TF-IDF Processor takes the indexed data from MongoDB, calculates the TF-IDF scores for each term in the documents, and stores the results back in MongoDB for fast retrieval by other services. This is essential for ranking search results based on the relevance of terms in the context of the entire collection of documents.

## Setup

### Using Docker

The recommended way to run the TF-IDF Processor is with Docker. This ensures all dependencies are handled and the service runs in an isolated environment.

1. **Install Docker**:  
   Follow the instructions for your OS on the [Docker website](https://docs.docker.com/get-docker/).

2. **Configure Environment Variables**:  
   Create a `variables.env` file in the `services/tfidf` directory with the following content (adjust as needed):
   ```env
   MONGO_HOST=<your_mongo_host>
   MONGO_PORT=<your_mongo_port>         # default: 27017
   MONGO_DB=<your_mongo_db>             # default: test
   MONGO_USERNAME=<your_mongo_username> # default: empty
   MONGO_PASSWORD=<your_mongo_password> # default: empty
   ```

3. **Build and Run**:  
   In the `services/tfidf` directory, run the following commands:
   ```bash
   docker-compose build
   docker-compose up
   ```

### Without Docker

If you prefer not to use Docker, you can run the TF-IDF Processor directly on your machine. Ensure you have all dependencies installed. It is recommended to use a virtual environment to avoid conflicts with other Python packages.

1. **Install Dependencies**:  
   Install the required packages using `pip`:
   ```bash
   pip install -r requirements.txt
   ```
2. **Configure Environment Variables**:  
   Set up the environment variables in your shell or create a `.env` file in the `services/tfidf` directory.
3. **Run the TF-IDF Processor**:  
   Execute the TF-IDF Processor script:
   ```bash
   python tfidf_processor.py
   ```
