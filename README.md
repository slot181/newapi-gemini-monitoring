# Gemini API 监控面板

这是一个简单的Web应用，用于监控连接的外部MySQL数据库中newapi `channels` 表的使用情况。它会显示每个渠道的分钟/天使用量和状态。

## 前提条件

*   Docker
*   Docker Compose
*   Newapi
*   MySQL

## 配置

1.  **数据库连接:**
    *   复制 `.env.example` 文件为 `.env`：`cp .env.example .env`
    *   编辑 `.env` 文件，填入您的外部MySQL数据库的连接信息 (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`)。
    *   您可以选择修改 `SERVER_PORT` 来更改监控面板的访问端口。

2.  **数据库更新逻辑 (存储过程):**
    *   此监控面板依赖一个名为 `UpdateChannelStats` 的MySQL存储过程来定期更新 `channels` 表中的使用数据和状态。
    *   **导入存储过程:** 使用您的MySQL客户端连接到数据库，并执行 `update_channels_procedure.sql` 文件中的内容。例如：
        ```bash
        mysql -h<DB_HOST> -P<DB_PORT> -u<DB_USER> -p'<DB_PASSWORD>' <DB_NAME> < update_channels_procedure.sql
        ```
        (请替换 `<...>` 占位符)。
    *   **设置定时执行:** 您需要设置一个定时任务来定期调用这个存储过程。推荐使用MySQL事件调度器或操作系统的cron。
        *   **示例 (Cron - 每分钟):**
            编辑crontab (`crontab -e`) 并添加（建议使用 `~/.my.cnf` 存储密码以提高安全性）：
            ```crontab
            * * * * * mysql -h<DB_HOST> -P<DB_PORT> -u<DB_USER> -p'<DB_PASSWORD>' <DB_NAME> -e "CALL UpdateChannelStats();" > /dev/null 2>&1
            ```

## 运行

1.  确保您已完成上述配置步骤。
2.  在项目根目录下运行：
    ```bash
    docker-compose up -d --build
    ```

## 访问

在浏览器中打开 `http://<您的服务器IP>:<SERVER_PORT>` (默认端口是 8080)。
