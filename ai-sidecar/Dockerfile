FROM python:3.11-slim

WORKDIR /app
ENV PYTHONUNBUFFERED=1 MODEL_DIR=/models

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ app/

EXPOSE 8901
# 容器内绑 0.0.0.0；宿主机经 compose 只发布 127.0.0.1:8901，外加 Bearer token
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8901"]
