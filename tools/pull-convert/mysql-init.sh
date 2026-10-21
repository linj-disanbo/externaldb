#!/bin/bash
# 启动包内 MySQL（数据/端口/密码都在 .env），并确保 schema 已应用。
# 生产可以不用：把 .env 的 [mysql].dsn 指向已有实例，MYSQL_LOCAL_START=false 即可。
. .env

MY_COMPOSE="docker-compose -f ./env/mysql/docker-compose.yaml"
MYSQL_SCHEMA="./env/mysql/sql/schema.sql"

# compose 文件在 env/mysql/ 下，docker-compose 从该目录读 .env，先同步一份
cp -f .env ./env/mysql/

exist_container=$(docker ps -a --format '{{.Names}}' | grep -w "${MYSQL_NAME}")
exist_port=$(netstat -tunlp 2>/dev/null | grep ":${MYSQL_PORT} ")

if [ -n "${exist_container}" ]; then
    echo "mysql 容器已存在（${MYSQL_NAME}），直接启动"
    ${MY_COMPOSE} up -d
elif [ -n "${exist_port}" ]; then
    echo "端口 ${MYSQL_PORT} 已被占用：如已有 MySQL，请把 .env 的 MYSQL_DSN 指向它并把 MYSQL_LOCAL_START=false"
    echo "退出运行"
    exit 1
else
    echo "mysql don't exist, to create mysql"
    mkdir -p "${MYSQL_DIR}"
    ${MY_COMPOSE} up -d
fi

echo "waiting mysql ready ..."
for i in $(seq 1 60); do
    if docker exec "${MYSQL_NAME}" mysqladmin ping -uroot -p"${MYSQL_ROOT_PASSWORD}" --silent >/dev/null 2>&1; then
        echo "mysql ready (${i}s)"
        break
    fi
    sleep 1s
done

if [ -f "${MYSQL_SCHEMA}" ]; then
    echo "apply schema: ${MYSQL_SCHEMA}"
    docker exec -i "${MYSQL_NAME}" mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" "${MYSQL_DATABASE}" < "${MYSQL_SCHEMA}" \
        && echo "schema ok" || echo "!! schema 应用失败，请手动执行"
else
    echo "!! 未找到 ${MYSQL_SCHEMA}，跳过 schema"
fi
