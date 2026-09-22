-- 楼主追记表：实际表结构由 GORM AutoMigrate 自动生成，此文件作为结构备案。
-- 业务规则（由 service/repository 层事务保证，非数据库约束）：
--   1. 同一帖子最多保留两条「已发布 + 待审核」追记；
--   2. 同一帖子至多一条「待审核」追记，并发提交时整次拒绝；
--   3. 追记一经发布不可修改，仅审核回调可将其在 pending/published/rejected 间流转；
--   4. 已驳回追记不占名额，review_note（驳回原因）仅作者本人可见。
CREATE TABLE IF NOT EXISTS `post_addendums` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `post_id` BIGINT UNSIGNED NOT NULL,
  `identity_id` BIGINT UNSIGNED NOT NULL,
  `content` VARCHAR(500) NOT NULL,
  `status` INT NOT NULL DEFAULT 1 COMMENT '1已发布 2待审核 3已驳回',
  `hit_words` VARCHAR(255) NOT NULL DEFAULT '',
  `reviewed_by` BIGINT UNSIGNED NULL,
  `review_note` VARCHAR(255) NOT NULL DEFAULT '',
  `reviewed_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NULL,
  `updated_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_post_addendums_post_id` (`post_id`),
  KEY `idx_post_addendums_identity_id` (`identity_id`),
  KEY `idx_post_addendums_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
