package pluginsdk

// Dialect 是 SDK SQL 代理使用的数据库方言。Plugin 初始化 ent 时
// 传给 entsql.OpenDB：
//
//	drv := entsql.OpenDB(pluginsdk.Dialect, ctx.DB())
//	client := ent.NewClient(ent.Driver(drv))
//
// 当前固定为 "postgres"。
const Dialect = "postgres"
