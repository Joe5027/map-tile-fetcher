# 五个区域缺口核查

核查时间 2026-09-11。结论：**0 项补齐，5 项已核实但未取得可靠完整几何**。
本次不新增或改写区域边界。行政公告确认名称/归属，不等同于可再分发的 WGS84 边界文件。
运行时说明与 CI 例外清单位于 `apps/admin-region-tiler/geojson/region-gaps.json`。

| 目录编码 | 名称 | 行政资料日期及来源 | 数据缺口与处理 |
| --- | --- | --- | --- |
| 460323 | 中沙群岛的岛礁及其海域 | [2020-04-18 三沙市设区报道，国防部转载新华社](https://www.mod.gov.cn/gfbw/qwfb/4863771.html) | 目录保留历史编码；现有行政资料涉及代管关系，不能据此改名或套用三沙市。未取得独立岛礁及海域完整几何。页面 reasonCode 为 `historical_boundary_unverified`。 |
| 653228 | 和康县 | [2024-12-27 设县公告，自治区人大转载自治区政府](https://www.xjpcsc.gov.cn/article/d45d1af202d8486ea035ecdfaf7fe9c7) | 公告确认名称及和田地区归属，未提供完整边界。保留 `authoritative_boundary_unavailable`。 |
| 653229 | 和安县 | [2024-12-27 自治区政府设县公告](https://www.xinjiang.gov.cn/xinjiang/tzgg/202412/a0b3b3ac363546589618dd9d49cc7fd7.shtml) | 公告确认名称及和田地区归属，未提供完整边界。保留 `authoritative_boundary_unavailable`。 |
| 659011 | 新星市 | [2021-02-04 设市通知及界线说明](https://www.xinjiang.gov.cn/xinjiang/zfgbml/202106/0f5b54112d0f43a59f9befc9e40d87da.shtml) | 有分段坐标和沿道路/农场走向，文件要求以协议附图为准；缺完整附图及坐标基准，不将转折点直接连线伪装完整边界。保留 `boundary_attachment_required`。 |
| 659012 | 白杨市 | [2023-01-20 自治区政府设市公告](https://www.xinjiang.gov.cn/xinjiang/tzgg/202301/8bd4c6578fae4c469680d068c187c7b0.shtml) | 确认名称及自治区直辖，未提供完整可用几何。保留 `authoritative_boundary_unavailable`。 |

## 原提供方核查

对五个编码逐项读取原工具的 DataV 地址：
`https://geo.datav.aliyun.com/areas_v3/bound/geojson?code=<编码>`，全部 HTTP 404。
同时核对 `https://geo.datav.aliyun.com/areas_v3/bound/<编码>.json`，全部 HTTP 404。
该只读调查不属于回归测试，CI 和 unittest 始终只用本地模拟服务，不重查外网。

数据日期：未取得边界，均记为 `unknown`；坐标系：`unverified`；使用条件：未取得可再分发边界数据。
不能把行政公告日期当作几何数据日期，也不能把经纬度数字自动当作 WGS84 证明。
[DataV 官方坐标文档](https://www.alibabacloud.com/help/zh/datav/datav-6-0/user-guide/map-data-format)
说明地图组件主要使用 GCJ-02，这进一步说明需要逐份核验边界坐标系；本轮不做猜测转换。
现存历史数据未在本轮宣称重新认证坐标系。

## 后续数据验收

取得来源及使用条件明确的完整 WGS84 边界后，先放入独立资源包，校验编码、父级、
全部 Feature、环、坐标范围和拓扑，再检查多区域预览与低层级本地模拟下载。
正式引入时同步移除缺口记录。现在保留目录条目、明确不可用状态，并阻止创建对应任务。
`check_region_catalog.py` 比较实际缺失集合与调查记录，新增缺失或过时例外均失败。
