# Geo Location 情况说明

日期：2026-10-01。状态：保存实现，暂停推进；以后再决定部署方案。

## 已确认的范围与实现

命令为 `dgs geo location`，是 CLI Action，不进入 TUI。
只做单次定位，明确不做 `--watch`。默认输出经纬度；支持
`--json/-j`、`--format/-f`、`--verbose/-v`、帮助及命令接口版本。
完整字段列表与语义见 [location.md](location.md)。

实现遵循 Reminders 的分层：Geo Action → Go 适配层
`internal/desktop/location` → cgo / Objective-C → Core Location 与 Contacts。
JSON 或地址类占位符触发反向地理编码。原生调用等待授权完成再启动定位，
十秒内结束，完成或超时都会停止定位、取消地址查询。非 macOS 或无 cgo
的构建明确报不支持。权限用途声明加入现有内嵌 plist；没有改变正式安装流程。

功能覆盖参考 CoreLocationCLI，但没有承诺逐字节兼容：JSON 优先于格式参数，
占位符替换顺序固定，返回值中的占位符不再展开；十秒超时也包含地址查询。
速度、精度等 JSON 值保持字符串，缺失字段为 null。

## 验证结果

macOS 原生包、Geo Action、CLI 测试通过；无 cgo 的对应测试也通过。
带内嵌 plist 的 macOS 二进制能够构建及签名，帮助与版本可用。
当前 SDK 对 CLGeocoder 有弃用提示，仍可编译；没有迁移到 MapKit。

本机系统为 macOS 26.6.2（25G83）。已实际观察：

| 对照 | 结果 |
| --- | --- |
| 原始单文件，嵌入用途声明并签名 | 无法取得定位授权 |
| 完整内嵌应用 plist、独立测试身份 | 超时；Launch Services 不接受裸可执行文件注册（-10811） |
| 去掉应用 ID 的内嵌 plist | 超时，同样没有弹窗 |
| 最小 Objective-C 单文件调用同一桥接 | 超时，失败可在 Go/cgo 之外复现 |
| 临时 dgs.app 包装与注册 | 获得授权，返回经纬度；完整 JSON、地址与时区也成功 |
| 临时应用授权后再运行原始裸二进制 | 仍被拒绝，不能直接复用应用授权 |

单文件测试的 CoreLocationAgent 日志明确出现：

```text
client bundle is NULL. Skip showing AuthPrompt
```

locationd 同时记录找不到对应 Launch Services 应用记录。
这些证据证明已测试的单文件方式在本机未走通，不能推断所有 macOS 版本、
所有可能方案都不支持单文件。

临时测试文件曾位于 `/tmp/dgs-geo-location`、
`/tmp/dgs-location-test/dgs.app`、`/tmp/dgs-single-test`、
`/tmp/dgs-single-noid` 和 `/tmp/dgs-location-native-probe`。
它们不是项目依赖，不纳入 Git，可能被系统清理。测试期间注册过临时应用并
授予其定位权限；没有重置权限数据库，也没有替换已安装的 dgs。

## 保留的约束与未决事项

用户希望继续保持单文件，认为 dgs.app 难以管理。正式安装仍只安装 dgs。
没有实现应用包装安装、后台 helper、自动生成隐藏应用或权限数据库修改。
功能代码已保存，但不能把标准单文件安装下的定位称作可用或已完成。

恢复工作时先寻找并验证符合单文件约束的授权方案；如确实需要改变部署约定，
再由用户决定。此前不合并为已完成的定位功能，也不替换正式安装方式。

可重复的代码检查：

```sh
go test ./internal/desktop/location ./internal/apps/geo ./internal/cli
CGO_ENABLED=0 go test ./internal/desktop/location ./internal/apps/geo ./internal/cli
```

参考：

- [CoreLocationCLI 实现](https://github.com/fulldecent/corelocationcli/blob/main/Sources/CoreLocationCLI/main.swift)
- [CoreLocationCLI 内嵌 plist 修复](https://github.com/fulldecent/corelocationcli/pull/53)
- [Apple 授权流程](https://developer.apple.com/documentation/corelocation/requesting-authorization-to-use-location-services)
