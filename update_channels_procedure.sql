-- 定义分隔符，因为存储过程内部有分号
DELIMITER //

-- 如果存在同名存储过程，则先删除
DROP PROCEDURE IF EXISTS UpdateChannelStats;

//

-- 创建存储过程，用于更新 channels 表的使用情况和状态
CREATE PROCEDURE UpdateChannelStats()
BEGIN
    -- 定义变量，存储1分钟前和今天早上8点的时间戳
    DECLARE minute_ago BIGINT;
    DECLARE day_start_8am BIGINT;

    -- 计算时间戳
    SET minute_ago = UNIX_TIMESTAMP(NOW() - INTERVAL 1 MINUTE);
    SET day_start_8am = UNIX_TIMESTAMP(
        CASE
            -- 如果当前时间是早上8点或之后，则取今天早上8点
            WHEN TIME(NOW()) >= '08:00:00'
            THEN TIMESTAMP(DATE(NOW()), '08:00:00')
            -- 如果当前时间在早上8点之前，则取昨天早上8点
            ELSE TIMESTAMP(DATE(NOW()) - INTERVAL 1 DAY, '08:00:00')
        END
    );

    -- 更新 channels 表 (移除数据库名前缀，使用当前数据库)
    UPDATE channels
    -- 左连接 logs 表，统计每个 channel 的使用次数
    LEFT JOIN (
        SELECT
            channel_id,
            -- 计算过去一分钟的日志数量
            SUM(created_at >= minute_ago) AS minute_count,
            -- 计算从早上8点开始的总日志数量
            COUNT(*) AS day_count
        FROM logs -- 移除数据库名前缀
        -- 只考虑从早上8点开始的日志
        WHERE created_at >= day_start_8am
        GROUP BY channel_id
    ) AS logs_stats ON channels.id = logs_stats.channel_id -- 移除数据库名前缀
    -- 设置更新的值
    SET
        -- 更新分钟使用次数，如果 logs_stats 中没有记录则为 0
        channels.count_minute_usage = IFNULL(logs_stats.minute_count, 0), -- 移除数据库名前缀
        -- 更新天使用次数，如果 logs_stats 中没有记录则为 0
        channels.count_day_usage = IFNULL(logs_stats.day_count, 0), -- 移除数据库名前缀
        -- 更新状态
        channels.status = CASE -- 移除数据库名前缀
            -- 如果是普号 (tag != 'gcp') 且分钟使用超限(>4) 或 天使用超限(>25)
            WHEN (channels.tag != 'gcp' AND (IFNULL(logs_stats.minute_count, 0) > 4 OR IFNULL(logs_stats.day_count, 0) > 25)) -- 移除数据库名前缀
            -- 或者 如果是付费号 (tag = 'gcp') 且分钟使用超限(>19) 或 天使用超限(>99)
            OR (channels.tag = 'gcp' AND (IFNULL(logs_stats.minute_count, 0) > 19 OR IFNULL(logs_stats.day_count, 0) > 99)) -- 移除数据库名前缀
            -- 则将状态设置为 2 (自动禁用)
            THEN 2
            -- 否则将状态设置为 1 (可用)
            ELSE 1
        END,
        -- 更新权重 (监控应用目前未使用此字段)
        channels.weight = CASE -- 移除数据库名前缀
            WHEN channels.tag = 'gcp' THEN GREATEST(1, 100 - IFNULL(logs_stats.day_count, 0)) -- 移除数据库名前缀
            ELSE GREATEST(1, 25 - IFNULL(logs_stats.day_count, 0))
        END;

END //

-- 将分隔符改回默认的分号
DELIMITER ;
