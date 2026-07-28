# Fork Release Notes

每个 TokenSupply fork release 都必须在创建 tag 前提交一份版本化说明：

```text
docs/releases/v<upstream-version>-ts.<revision>.md
```

Release workflow 从 tag 指向的 commit 直接读取该文件。Annotated tag message 只作为简短标签，不再承载发布说明。

使用 `TEMPLATE.md` 建立新文件，并删除所有占位文本。发布说明面向管理员和用户，不能用原始 commit 列表代替。
