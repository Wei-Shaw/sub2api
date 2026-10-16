/**
 * 「AI生图」一条生成记录的类型。
 *
 * 为什么单独放一个文件：结果网格被抽成了子组件
 * （`components/aiImage/AIImageResultGrid.vue`），视图和子组件都要用这个类型。
 * 类型定义在 .vue 里没法被两边共享，所以提到这里。
 */

export type RecordStatus = 'generating' | 'completed' | 'failed'

/**
 * 这条记录是哪个功能做出来的。
 *
 * 主人要求：每个功能页里只显示**这个功能自己**的结果，
 * 页面最下面的「生图结果」才汇总全部。所以每条记录都要记住出处。
 *
 * 注意：这个字段是后加的，升级前存下来的老记录没有它。
 * 老记录推断不出出处，因此**只出现在最下面的汇总区**，
 * 不会出现在任何功能页里（宁可少显示，也不要显示错的）。
 */
export type RecordSource = 'single' | 'hot' | 'script'

export interface AIImageRecord {
  id: string
  prompt: string
  model: string
  size: string
  status: RecordStatus
  images: string[]
  error: string
  createdAt: number
  /** 生成时是否开启了商品一致性锁定（重试时要还原成同样的输入）。 */
  locked?: boolean
  /** 对应的商品编号；客户端按 images/<product_id>/<index>.jpg 认图 */
  productId?: string
  /**
   * 提交顺序，单调递增。
   *
   * 为什么不能只用 createdAt：批量生成时几个请求是并发发出去的，
   * createdAt 可能撞在同一毫秒，排序就不稳定了。
   * 而客户端是按地址的先后顺序落成第 1、2、3 张的。
   * 顺序一乱，上架时主图顺序就错了。所以用这个字段兜住顺序。
   */
  seq?: number
  /**
   * 第几轮生成。
   *
   * 主人要求「进行下一个图片处理的时候，不要显示之前进行的图片」，
   * 定的做法是：**旧图保留，但当前这轮排在最前面**。
   * 一次批量（或一次单张生成）= 一轮，同一轮的记录共享同一个 roundId。
   * 结果区据此分成「本轮生成」和「历史记录」两段。
   */
  roundId?: number
  /** 哪个功能做出来的；老记录没有这个字段。 */
  source?: RecordSource
}

/** 结果区的一行：同一轮的记录排在同一行里横向流动。 */
export interface ResultRow {
  key: string
  records: AIImageRecord[]
}

/** 结果区的一段（本轮 / 历史），里面按轮次切成若干行。 */
export interface ResultSection {
  key: string
  title: string
  rows: ResultRow[]
}
