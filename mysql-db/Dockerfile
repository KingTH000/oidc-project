FROM mysql:8.0

COPY blog.sql /docker-entrypoint-initdb.d/blog.sql

ENV MYSQL_ROOT_PASSWORD=your_secret_password
ENV MYSQL_DATABASE=blog