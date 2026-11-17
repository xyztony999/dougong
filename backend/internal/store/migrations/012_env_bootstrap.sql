-- 012_env_bootstrap.sql（env-mgmt v1.2 安装后配置就绪增量，api-env-mgmt.md D-18/5.3）
-- env_install_task +引导结果两列：bootstrap_status（独立结果维度，NULL=无引导步骤）
-- + bootstrap_note（引导摘要/原因，不含密码）。任务 status 仍为包操作四态结果，
-- 引导失败不改 succeeded（⚠️A14）。新增可空列用 ALTER TABLE ADD COLUMN（SQLite
-- 支持带 CHECK 的可空列追加，无需表重建）；既有行两列 NULL（无引导步骤），无数据丢失。
ALTER TABLE env_install_task ADD COLUMN bootstrap_status TEXT CHECK (bootstrap_status IN ('succeeded','failed','skipped'));
ALTER TABLE env_install_task ADD COLUMN bootstrap_note TEXT;