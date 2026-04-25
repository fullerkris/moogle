FROM python:3.12-alpine

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY exporter.py ./

EXPOSE 9108

CMD ["python", "/app/exporter.py"]
