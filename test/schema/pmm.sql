DROP DATABASE IF EXISTS dev_pmm;
CREATE DATABASE dev_pmm;
USE dev_pmm;

SET time_zone='+00:00'; -- be sure we use right timezone otherwise dates like '1970-01-01 00:00:01' might become incorrect

CREATE TABLE IF NOT EXISTS instances (
  instance_id   INT UNSIGNED NOT NULL AUTO_INCREMENT,
  subsystem_id  INT UNSIGNED NOT NULL, -- 1=os, 2=agent, 3=mysql
  parent_uuid   CHAR(32) NULL,
  uuid          CHAR(32) NOT NULL,
  name          VARCHAR(100) CHARSET 'utf8' NOT NULL,
  dsn           VARCHAR(500) CHARSET 'utf8' NULL,  -- for addressable subsystems, like MySQL
  distro        VARCHAR(100) CHARSET 'utf8' NULL,
  version       VARCHAR(50)  CHARSET 'utf8' NULL,
  created       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted       TIMESTAMP NULL DEFAULT NULL,
  PRIMARY KEY (instance_id),
  UNIQUE INDEX (uuid),
  UNIQUE INDEX (name, subsystem_id, deleted)
);

CREATE TABLE IF NOT EXISTS agent_configs (
  agent_instance_id  INT UNSIGNED NOT NULL,
  service            VARCHAR(10) NOT NULL,            -- agent, log, data, it, qan, mm, sysconfig
  other_instance_id  INT UNSIGNED NOT NULL DEFAULT 0, -- if service uses another instance (qan, mm)
  in_file            TEXT NOT NULL,                   -- JSON, aka "set" but this is reserved word
  running            TEXT NOT NULL,                   -- JSON
  updated            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (agent_instance_id, service, other_instance_id)
);

CREATE TABLE IF NOT EXISTS agent_log (
  instance_id    INT UNSIGNED NOT NULL,
  sec            BIGINT UNSIGNED NOT NULL,     -- Unix timestamp (seconds)
  nsec           BIGINT UNSIGNED NOT NULL,     -- nanoseconds for ^
  level          TINYINT(1) UNSIGNED NOT NULL, -- 7 debug, 6 info, ...
  service        VARCHAR(50) NOT NULL,         -- service + instance name, e.g. mm-mysql-db01
  msg            VARCHAR(5000) CHARSET 'utf8' NOT NULL,
  INDEX (instance_id, sec, level)
);

CREATE TABLE IF NOT EXISTS query_classes (
  query_class_id  INT UNSIGNED NOT NULL AUTO_INCREMENT,
  --
  checksum           CHAR(32) NOT NULL,         -- F9CA3F38A5D4C05B
  abstract           VARCHAR(100) DEFAULT NULL, -- SELECT t
  fingerprint        VARCHAR(5000) NOT NULL,    -- select * from t where id=?
  tables             TEXT DEFAULT NULL,
  first_seen         TIMESTAMP NULL DEFAULT NULL,
  last_seen          TIMESTAMP NULL DEFAULT NULL,
  status             CHAR(3) NOT NULL DEFAULT 'new',
  --
  PRIMARY KEY (query_class_id),
  UNIQUE INDEX (checksum)
) CHARSET='utf8';

CREATE TABLE IF NOT EXISTS query_examples (
  query_class_id  INT UNSIGNED NOT NULL,           -- PK
  instance_id     INT UNSIGNED NOT NULL DEFAULT 0, -- PK
  period          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, -- PK
  ts              TIMESTAMP NULL DEFAULT NULL,
  db              VARCHAR(255) NOT NULL DEFAULT '',
  Query_time      FLOAT NOT NULL DEFAULT 0,
  query           TEXT NOT NULL,
  --
  PRIMARY KEY (query_class_id, instance_id, period)
) CHARSET='utf8';

CREATE TABLE IF NOT EXISTS query_global_metrics (
  instance_id              INT UNSIGNED NOT NULL, -- PK
  start_ts                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,    -- PK
  end_ts                   TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:01',
  run_time                 FLOAT,
  total_query_count        BIGINT UNSIGNED NOT NULL,
  unique_query_count       BIGINT UNSIGNED NOT NULL,  -- number of query classes
  -- Log file
  rate_type                VARCHAR(10),            -- session or query (PS)
  rate_limit               SMALLINT UNSIGNED,      -- every Nth rate_type (PS)
  log_file                 VARCHAR(100),             -- slow_query_log_file
  log_file_size            BIGINT UNSIGNED,
  start_offset             BIGINT UNSIGNED,          -- in log_file
  end_offset               BIGINT UNSIGNED,          -- in log_file
  stop_offset              BIGINT UNSIGNED,          -- in log_file
  -- Metrics
  Query_time_sum       FLOAT,
  Query_time_min       FLOAT,
  Query_time_max       FLOAT,
  Query_time_avg       FLOAT,
  Query_time_p95       FLOAT,
  Query_time_stddev    FLOAT,
  Query_time_med       FLOAT,
  Lock_time_sum        FLOAT,
  Lock_time_min        FLOAT,
  Lock_time_max        FLOAT,
  Lock_time_avg        FLOAT,
  Lock_time_p95        FLOAT,
  Lock_time_stddev     FLOAT,
  Lock_time_med        FLOAT,
  Rows_sent_sum        BIGINT UNSIGNED,
  Rows_sent_min        BIGINT UNSIGNED,
  Rows_sent_max        BIGINT UNSIGNED,
  Rows_sent_avg        BIGINT UNSIGNED,
  Rows_sent_p95        BIGINT UNSIGNED,
  Rows_sent_stddev     BIGINT UNSIGNED,
  Rows_sent_med        BIGINT UNSIGNED,
  Rows_examined_sum    BIGINT UNSIGNED,
  Rows_examined_min    BIGINT UNSIGNED,
  Rows_examined_max    BIGINT UNSIGNED,
  Rows_examined_avg    BIGINT UNSIGNED,
  Rows_examined_p95    BIGINT UNSIGNED,
  Rows_examined_stddev BIGINT UNSIGNED,
  Rows_examined_med    BIGINT UNSIGNED,
  -- Percona extended slowlog attributes
  -- http://www.percona.com/docs/wiki/patches:slow_extended
  Rows_affected_sum             BIGINT UNSIGNED,
  Rows_affected_min             BIGINT UNSIGNED,
  Rows_affected_max             BIGINT UNSIGNED,
  Rows_affected_avg             BIGINT UNSIGNED,
  Rows_affected_p95             BIGINT UNSIGNED,
  Rows_affected_stddev          BIGINT UNSIGNED,
  Rows_affected_med             BIGINT UNSIGNED,
  Rows_read_sum                 BIGINT UNSIGNED,
  Rows_read_min                 BIGINT UNSIGNED,
  Rows_read_max                 BIGINT UNSIGNED,
  Rows_read_avg                 BIGINT UNSIGNED,
  Rows_read_p95                 BIGINT UNSIGNED,
  Rows_read_stddev              BIGINT UNSIGNED,
  Rows_read_med                 BIGINT UNSIGNED,
  Merge_passes_sum              BIGINT UNSIGNED,
  Merge_passes_min              BIGINT UNSIGNED,
  Merge_passes_max              BIGINT UNSIGNED,
  Merge_passes_avg              BIGINT UNSIGNED,
  Merge_passes_p95              BIGINT UNSIGNED,
  Merge_passes_stddev           BIGINT UNSIGNED,
  Merge_passes_med              BIGINT UNSIGNED,
  InnoDB_IO_r_ops_sum           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_min           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_max           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_avg           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_p95           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_stddev        BIGINT UNSIGNED,
  InnoDB_IO_r_ops_med           BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_sum         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_min         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_max         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_avg         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_p95         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_stddev      BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_med         BIGINT UNSIGNED,
  InnoDB_IO_r_wait_sum          FLOAT,
  InnoDB_IO_r_wait_min          FLOAT,
  InnoDB_IO_r_wait_max          FLOAT,
  InnoDB_IO_r_wait_avg          FLOAT,
  InnoDB_IO_r_wait_p95          FLOAT,
  InnoDB_IO_r_wait_stddev       FLOAT,
  InnoDB_IO_r_wait_med          FLOAT,
  InnoDB_rec_lock_wait_sum      FLOAT,
  InnoDB_rec_lock_wait_min      FLOAT,
  InnoDB_rec_lock_wait_max      FLOAT,
  InnoDB_rec_lock_wait_avg      FLOAT,
  InnoDB_rec_lock_wait_p95      FLOAT,
  InnoDB_rec_lock_wait_stddev   FLOAT,
  InnoDB_rec_lock_wait_med      FLOAT,
  InnoDB_queue_wait_sum         FLOAT,
  InnoDB_queue_wait_min         FLOAT,
  InnoDB_queue_wait_max         FLOAT,
  InnoDB_queue_wait_avg         FLOAT,
  InnoDB_queue_wait_p95         FLOAT,
  InnoDB_queue_wait_stddev      FLOAT,
  InnoDB_queue_wait_med         FLOAT,
  InnoDB_pages_distinct_sum     BIGINT UNSIGNED,
  InnoDB_pages_distinct_min     BIGINT UNSIGNED,
  InnoDB_pages_distinct_max     BIGINT UNSIGNED,
  InnoDB_pages_distinct_avg     BIGINT UNSIGNED,
  InnoDB_pages_distinct_p95     BIGINT UNSIGNED,
  InnoDB_pages_distinct_stddev  BIGINT UNSIGNED,
  InnoDB_pages_distinct_med     BIGINT UNSIGNED,
  -- Yes/No metrics
  -- sum/global.total_query_count = % true
  QC_Hit_sum                    BIGINT UNSIGNED,
  Full_scan_sum                 BIGINT UNSIGNED,
  Full_join_sum                 BIGINT UNSIGNED,
  Tmp_table_sum                 BIGINT UNSIGNED,
  Tmp_table_on_disk_sum         BIGINT UNSIGNED,
  Filesort_sum                  BIGINT UNSIGNED,
  Filesort_on_disk_sum          BIGINT UNSIGNED,
  -- Meta metrics
  Query_length_sum              BIGINT UNSIGNED,
  Query_length_min              BIGINT UNSIGNED,
  Query_length_max              BIGINT UNSIGNED,
  Query_length_avg              BIGINT UNSIGNED,
  Query_length_p95              BIGINT UNSIGNED,
  Query_length_stddev           BIGINT UNSIGNED,
  Query_length_med              BIGINT UNSIGNED,
  -- Memory footprint metrics
  Bytes_sent_sum                BIGINT UNSIGNED,
  Bytes_sent_min                BIGINT UNSIGNED,
  Bytes_sent_max                BIGINT UNSIGNED,
  Bytes_sent_avg                BIGINT UNSIGNED,
  Bytes_sent_p95                BIGINT UNSIGNED,
  Bytes_sent_stddev             BIGINT UNSIGNED,
  Bytes_sent_med                BIGINT UNSIGNED,
  Tmp_tables_sum                BIGINT UNSIGNED,
  Tmp_tables_min                BIGINT UNSIGNED,
  Tmp_tables_max                BIGINT UNSIGNED,
  Tmp_tables_avg                BIGINT UNSIGNED,
  Tmp_tables_p95                BIGINT UNSIGNED,
  Tmp_tables_stddev             BIGINT UNSIGNED,
  Tmp_tables_med                BIGINT UNSIGNED,
  Tmp_disk_tables_sum           BIGINT UNSIGNED,
  Tmp_disk_tables_min           BIGINT UNSIGNED,
  Tmp_disk_tables_max           BIGINT UNSIGNED,
  Tmp_disk_tables_avg           BIGINT UNSIGNED,
  Tmp_disk_tables_p95           BIGINT UNSIGNED,
  Tmp_disk_tables_stddev        BIGINT UNSIGNED,
  Tmp_disk_tables_med           BIGINT UNSIGNED,
  Tmp_table_sizes_sum           BIGINT UNSIGNED,
  Tmp_table_sizes_min           BIGINT UNSIGNED,
  Tmp_table_sizes_max           BIGINT UNSIGNED,
  Tmp_table_sizes_avg           BIGINT UNSIGNED,
  Tmp_table_sizes_p95           BIGINT UNSIGNED,
  Tmp_table_sizes_stddev        BIGINT UNSIGNED,
  Tmp_table_sizes_med           BIGINT UNSIGNED,
  -- Performance Schema
  Errors_sum                    BIGINT UNSIGNED,
  Warnings_sum                  BIGINT UNSIGNED,
  Select_full_range_join_sum    BIGINT UNSIGNED,
  Select_range_sum              BIGINT UNSIGNED,
  Select_range_check_sum        BIGINT UNSIGNED,
  Sort_range_sum                BIGINT UNSIGNED,
  Sort_rows_sum                 BIGINT UNSIGNED,
  Sort_scan_sum                 BIGINT UNSIGNED,
  No_index_used_sum             BIGINT UNSIGNED,
  No_good_index_used_sum        BIGINT UNSIGNED,
  --
  PRIMARY KEY (instance_id, start_ts),
  INDEX (start_ts)
);

CREATE TABLE IF NOT EXISTS query_class_metrics (
  query_class_id           INT UNSIGNED NOT NULL, -- PK
  instance_id              INT UNSIGNED NOT NULL, -- PK
  start_ts                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,    -- PK
  end_ts                   TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:01',
  query_count              BIGINT UNSIGNED NOT NULL,
  lrq_count                BIGINT UNSIGNED NOT NULL DEFAULT 0,  -- Low-ranking Queries
  -- Metrics
  Query_time_sum                FLOAT,
  Query_time_min                FLOAT,
  Query_time_max                FLOAT,
  Query_time_avg                FLOAT,
  Query_time_p95                FLOAT,
  Query_time_stddev             FLOAT,
  Query_time_med                FLOAT,
  Lock_time_sum                 FLOAT,
  Lock_time_min                 FLOAT,
  Lock_time_max                 FLOAT,
  Lock_time_avg                 FLOAT,
  Lock_time_p95                 FLOAT,
  Lock_time_stddev              FLOAT,
  Lock_time_med                 FLOAT,
  Rows_sent_sum                 BIGINT UNSIGNED,
  Rows_sent_min                 BIGINT UNSIGNED,
  Rows_sent_max                 BIGINT UNSIGNED,
  Rows_sent_avg                 BIGINT UNSIGNED,
  Rows_sent_p95                 BIGINT UNSIGNED,
  Rows_sent_stddev              BIGINT UNSIGNED,
  Rows_sent_med                 BIGINT UNSIGNED,
  Rows_examined_sum             BIGINT UNSIGNED,
  Rows_examined_min             BIGINT UNSIGNED,
  Rows_examined_max             BIGINT UNSIGNED,
  Rows_examined_avg             BIGINT UNSIGNED,
  Rows_examined_p95             BIGINT UNSIGNED,
  Rows_examined_stddev          BIGINT UNSIGNED,
  Rows_examined_med             BIGINT UNSIGNED,
  -- Percona extended slowlog attributes
  -- http://www.percona.com/docs/wiki/patches:slow_extended
  Rows_affected_sum             BIGINT UNSIGNED,
  Rows_affected_min             BIGINT UNSIGNED,
  Rows_affected_max             BIGINT UNSIGNED,
  Rows_affected_avg             BIGINT UNSIGNED,
  Rows_affected_p95             BIGINT UNSIGNED,
  Rows_affected_stddev          BIGINT UNSIGNED,
  Rows_affected_med             BIGINT UNSIGNED,
  Rows_read_sum                 BIGINT UNSIGNED,
  Rows_read_min                 BIGINT UNSIGNED,
  Rows_read_max                 BIGINT UNSIGNED,
  Rows_read_avg                 BIGINT UNSIGNED,
  Rows_read_p95                 BIGINT UNSIGNED,
  Rows_read_stddev              BIGINT UNSIGNED,
  Rows_read_med                 BIGINT UNSIGNED,
  Merge_passes_sum              BIGINT UNSIGNED,
  Merge_passes_min              BIGINT UNSIGNED,
  Merge_passes_max              BIGINT UNSIGNED,
  Merge_passes_avg              BIGINT UNSIGNED,
  Merge_passes_p95              BIGINT UNSIGNED,
  Merge_passes_stddev           BIGINT UNSIGNED,
  Merge_passes_med              BIGINT UNSIGNED,
  InnoDB_IO_r_ops_sum           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_min           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_max           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_avg           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_p95           BIGINT UNSIGNED,
  InnoDB_IO_r_ops_stddev        BIGINT UNSIGNED,
  InnoDB_IO_r_ops_med           BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_sum         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_min         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_max         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_avg         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_p95         BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_stddev      BIGINT UNSIGNED,
  InnoDB_IO_r_bytes_med         BIGINT UNSIGNED,
  InnoDB_IO_r_wait_sum          FLOAT,
  InnoDB_IO_r_wait_min          FLOAT,
  InnoDB_IO_r_wait_max          FLOAT,
  InnoDB_IO_r_wait_avg          FLOAT,
  InnoDB_IO_r_wait_p95          FLOAT,
  InnoDB_IO_r_wait_stddev       FLOAT,
  InnoDB_IO_r_wait_med          FLOAT,
  InnoDB_rec_lock_wait_sum      FLOAT,
  InnoDB_rec_lock_wait_min      FLOAT,
  InnoDB_rec_lock_wait_max      FLOAT,
  InnoDB_rec_lock_wait_avg      FLOAT,
  InnoDB_rec_lock_wait_p95      FLOAT,
  InnoDB_rec_lock_wait_stddev   FLOAT,
  InnoDB_rec_lock_wait_med      FLOAT,
  InnoDB_queue_wait_sum         FLOAT,
  InnoDB_queue_wait_min         FLOAT,
  InnoDB_queue_wait_max         FLOAT,
  InnoDB_queue_wait_avg         FLOAT,
  InnoDB_queue_wait_p95         FLOAT,
  InnoDB_queue_wait_stddev      FLOAT,
  InnoDB_queue_wait_med         FLOAT,
  InnoDB_pages_distinct_sum     BIGINT UNSIGNED,
  InnoDB_pages_distinct_min     BIGINT UNSIGNED,
  InnoDB_pages_distinct_max     BIGINT UNSIGNED,
  InnoDB_pages_distinct_avg     BIGINT UNSIGNED,
  InnoDB_pages_distinct_p95     BIGINT UNSIGNED,
  InnoDB_pages_distinct_stddev  BIGINT UNSIGNED,
  InnoDB_pages_distinct_med     BIGINT UNSIGNED,
  -- Yes/No metrics
  -- sum/class.query_count = % true
  QC_Hit_sum                    BIGINT UNSIGNED,
  Full_scan_sum                 BIGINT UNSIGNED,
  Full_join_sum                 BIGINT UNSIGNED,
  Tmp_table_sum                 BIGINT UNSIGNED,
  Tmp_table_on_disk_sum         BIGINT UNSIGNED,
  Filesort_sum                  BIGINT UNSIGNED,
  Filesort_on_disk_sum          BIGINT UNSIGNED,
  -- Meta metrics
  Query_length_sum              BIGINT UNSIGNED,
  Query_length_min              BIGINT UNSIGNED,
  Query_length_max              BIGINT UNSIGNED,
  Query_length_avg              BIGINT UNSIGNED,
  Query_length_p95              BIGINT UNSIGNED,
  Query_length_stddev           BIGINT UNSIGNED,
  Query_length_med              BIGINT UNSIGNED,
  -- Memory footprint metrics
  Bytes_sent_sum                BIGINT UNSIGNED,
  Bytes_sent_min                BIGINT UNSIGNED,
  Bytes_sent_max                BIGINT UNSIGNED,
  Bytes_sent_avg                BIGINT UNSIGNED,
  Bytes_sent_p95                BIGINT UNSIGNED,
  Bytes_sent_stddev             BIGINT UNSIGNED,
  Bytes_sent_med                BIGINT UNSIGNED,
  Tmp_tables_sum                BIGINT UNSIGNED,
  Tmp_tables_min                BIGINT UNSIGNED,
  Tmp_tables_max                BIGINT UNSIGNED,
  Tmp_tables_avg                BIGINT UNSIGNED,
  Tmp_tables_p95                BIGINT UNSIGNED,
  Tmp_tables_stddev             BIGINT UNSIGNED,
  Tmp_tables_med                BIGINT UNSIGNED,
  Tmp_disk_tables_sum           BIGINT UNSIGNED,
  Tmp_disk_tables_min           BIGINT UNSIGNED,
  Tmp_disk_tables_max           BIGINT UNSIGNED,
  Tmp_disk_tables_avg           BIGINT UNSIGNED,
  Tmp_disk_tables_p95           BIGINT UNSIGNED,
  Tmp_disk_tables_stddev        BIGINT UNSIGNED,
  Tmp_disk_tables_med           BIGINT UNSIGNED,
  Tmp_table_sizes_sum           BIGINT UNSIGNED,
  Tmp_table_sizes_min           BIGINT UNSIGNED,
  Tmp_table_sizes_max           BIGINT UNSIGNED,
  Tmp_table_sizes_avg           BIGINT UNSIGNED,
  Tmp_table_sizes_p95           BIGINT UNSIGNED,
  Tmp_table_sizes_stddev        BIGINT UNSIGNED,
  Tmp_table_sizes_med           BIGINT UNSIGNED,
  -- Performance Schema
  Errors_sum                    BIGINT UNSIGNED,
  Warnings_sum                  BIGINT UNSIGNED,
  Select_full_range_join_sum    BIGINT UNSIGNED,
  Select_range_sum              BIGINT UNSIGNED,
  Select_range_check_sum        BIGINT UNSIGNED,
  Sort_range_sum                BIGINT UNSIGNED,
  Sort_rows_sum                 BIGINT UNSIGNED,
  Sort_scan_sum                 BIGINT UNSIGNED,
  No_index_used_sum             BIGINT UNSIGNED,
  No_good_index_used_sum        BIGINT UNSIGNED,
  --
  PRIMARY KEY (query_class_id, instance_id, start_ts),
  INDEX (start_ts)
);
