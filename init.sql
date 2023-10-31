use godycdn;
CREATE TABLE `object`
(
    `id`               bigint unsigned                  NOT NULL AUTO_INCREMENT,
    `hash`             char(64) COLLATE utf8_unicode_ci NOT NULL,
    `is_stored`        tinyint(1)                       NOT NULL DEFAULT '0',
    `length`           bigint unsigned                           DEFAULT NULL,
    `last_accessed_at` timestamp                        NULL     DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `id` (`id`),
    UNIQUE KEY `hash_idx` (`hash`),
    KEY `last_accessed_idx` (`last_accessed_at`),
    KEY `is_stored_idx` (`is_stored`)
);