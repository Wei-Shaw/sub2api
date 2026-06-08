package ent

import "entgo.io/ent/dialect"

// Driver 暴露底层 driver，供需要 raw SQL 的集成层使用。
func (c *Client) Driver() dialect.Driver {
	return c.driver
}
