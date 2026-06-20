package rag

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// dbMessage 从 t_chatmsg 读出的一条文本消息。
type dbMessage struct {
	ID       int64
	SenderID int32
	TargetID int32
	Content  string
	Ts       int64
}

// OpenMySQL 打开 MySQL 连接(只读用途)。dsn 形如：
//   user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=true
func OpenMySQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 失败: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	return db, nil
}

// loadUserTextMessages 拉取与某用户相关的【文本】消息(f_msgtype=0)，按时间倒序取最近 limit 条。
// 媒体消息(语音/视频等)无可检索文本，跳过(需要的话可结合 ASR 转写后再纳入)。
func loadUserTextMessages(ctx context.Context, db *sql.DB, owner int32, limit int) ([]dbMessage, error) {
	const q = `SELECT f_id, f_senderid, f_targetid, f_msgcontent, UNIX_TIMESTAMP(f_create_time)
	           FROM t_chatmsg
	           WHERE (f_senderid = ? OR f_targetid = ?) AND f_msgtype = 0
	           ORDER BY f_id DESC
	           LIMIT ?`
	rows, err := db.QueryContext(ctx, q, owner, owner, limit)
	if err != nil {
		return nil, fmt.Errorf("查询聊天记录失败: %w", err)
	}
	defer rows.Close()

	var out []dbMessage
	for rows.Next() {
		var m dbMessage
		var content []byte // f_msgcontent 是 blob
		if err := rows.Scan(&m.ID, &m.SenderID, &m.TargetID, &content, &m.Ts); err != nil {
			return nil, fmt.Errorf("扫描行失败: %w", err)
		}
		m.Content = string(content)
		out = append(out, m)
	}
	return out, rows.Err()
}
