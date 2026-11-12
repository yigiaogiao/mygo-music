# 上架懒猫商店：一步一步

从 0.1.36 开始，MyGo 音乐以包 ID `cloud.lazycat.app.mygomusic` 提交商店。
自用侧载和商店安装用的是同一个包 ID，所以**不要把同一台微服上的侧载版和商店版
当成两个应用**：从商店安装会直接覆盖/升级侧载版，数据在 `/lzcapp/var/mygomusic`，
不受影响。

需要的材料都在这个目录：

| 材料 | 文件 |
| --- | --- |
| 安装包 | `lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.39.lpk`（构建后生成） |
| 图标 | `lazycat/icon.png`（512×512 PNG，约 190 KB） |
| 桌面截图 | `lazycat/store/screenshots/pc/01-home.png` … `04-settings.png`（1920×1080，16:9） |
| 手机截图 | `lazycat/store/screenshots/mobile/01-home.png` … `04-now-playing.png`（720×405，16:9） |
| 更新日志 | `lazycat/store/changelog.zh-CN.txt`、`changelog.en-US.txt` |

---

## 第 0 步：确认安装包

```bash
cd ~/orca/projects/mygo-music
./scripts/build-lpk.sh
~/.local/share/lzctools/node_modules/.bin/lzc-cli project lint lazycat -f lazycat/lzc-build.yml
```

`project lint` 应该没有 error。再看一眼包信息：

```bash
~/.local/share/lzctools/node_modules/.bin/lzc-cli lpk info \
  lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.39.lpk
```

重点确认：`package`、`version`、`icon` 都在，镜像只有官方运行时
`registry.lazycat.cloud/lzc/lzcapp:3.20.3`（在 `registry.lazycat.cloud` 上，
所以**不需要** `lzc-cli appstore copy-image`）。

## 第 1 步：登录开发者账号

这一步是交互式的，需要你自己在终端里执行（在 Claude Code 里用 `!` 前缀）：

```bash
! ~/.local/share/lzctools/node_modules/.bin/lzc-cli appstore login
```

按提示输入开发者账号和密码。也可以用 `-u <用户名> -p <密码>` 一次给全，但密码会
留在 shell 历史里，不推荐。登录成功后 token 存在 lzc-cli 的配置里，后续
`publish` 会自动复用。

## 第 2 步：在开发者中心创建应用

打开 <https://developer.lazycat.cloud/manage>，用同一个开发者账号登录，点「新增」。

按下面填写（名称/简介等以 LPK 为准，网页上锁定的字段不用改）：

| 字段 | 填什么 |
| --- | --- |
| 应用名称 | `MyGo 音乐`（中文）/ `MyGo Music`（英文） |
| 包标识符 / package | `cloud.lazycat.app.mygomusic` |
| 简介（brief） | 由 LPK 带出，网页上不可编辑 |
| 描述（description） | 见下方「描述文案」 |
| 关键词 | `音乐,播放器,网盘,本地音乐,歌词,无损,music,player,lyrics` |
| 原创开发 / 本人是源作者 | **不要勾**：MyGo 的原作者是 egoist，这是适配懒猫的修改版 |
| 源作者或组织 | `egoist` |
| 来源（source） | `https://github.com/egoist/mygo-music`（原项目仓库） |
| 支持平台 | 桌面端 ✓、移动端 ✓（两个都勾） |
| 图标 | 上传 `lazycat/icon.png` |
| 桌面截图 | 上传 `lazycat/store/screenshots/pc/` 里的 4 张 |
| 手机截图 | 上传 `lazycat/store/screenshots/mobile/` 里的 4 张 |

截图规格（平台会校验）：

- 桌面：至少 2 张、最多 8 张，PNG/JPEG，单张 ≤15 MB，16:9，每边 1080–7680 px。
- 手机：勾了「支持移动端」就至少 3 张、最多 8 张，PNG/JPEG，单张 ≤15 MB，
  16:9，每边 320–3840 px。
- 图标：只收 PNG，≤200 KB，1:1。

分类由平台/审核设置，开发者目前不能自己选，不用管。

### 描述文案

中文：

```text
MyGo 音乐播放懒猫网盘里的音乐，不会复制或改动你的文件。

- 支持 MP3、FLAC、AAC、M4A、OGG、WAV
- 优先读取歌曲旁边的同名 .lrc；没有歌词的歌自动到 LRCLib 在线寻找
- 按歌曲、专辑、艺人浏览，支持歌单、喜欢的歌和搜索
- 桌面端和手机端都能用，手机端为窄屏重新排布
- 歌单、喜欢和扫描缓存保存在微服本地，重启和升级不丢失

第一次打开时点「设置 → 音乐来源 → 选择文件夹」，在网盘里选中放音乐的目录即可。

本应用基于 egoist 的 MyGo Music（AGPL-3.0）修改并适配懒猫微服，修改版源码：
https://github.com/yigiaogiao/mygo-music
```

English：

```text
MyGo Music plays the music in your LazyCat netdisk without copying or changing your files.

- MP3, FLAC, AAC, M4A, OGG and WAV
- Reads a song's .lrc file when there is one; songs without lyrics find them at LRCLib
- Browse by song, album and artist, with playlists, favorites and search
- Works on desktop and mobile, with a layout made for narrow screens
- Playlists, favorites and the scan cache stay on your microserver across restarts and upgrades

On first launch, open Settings → Music Sources → Choose Folders and pick the folder that holds your music.

This app is based on MyGo Music by egoist (AGPL-3.0), modified for LazyCat. Modified source:
https://github.com/yigiaogiao/mygo-music
```

### 关于「原创开发」勾选框

勾上表示「原创开发或本人是源作者」，同时会清空来源和源作者两个字段。MyGo 的
原作者是 egoist，这个包是在其 AGPL-3.0 代码上做的懒猫适配，**不能勾**。不勾时
网页会要求填写「源作者或组织名称」和「移植项目主页或代码仓库地址」，按上表填
`egoist` 和原仓库地址即可；修改版源码放在描述里，满足 AGPL 的源码可得要求。

## 第 3 步：在开发者中心上传版本并提交审核（推荐，0.1.36 实际走的路径）

网页流程比 CLI 直观，0.1.36 就是这么提交的：

1. 应用管理 → **编辑更新应用** → 右上角 **上传新版本**，选 `.lpk`。传完后
   包标识符、应用版本、最小系统版本、SHA256 会自动带出。
2. **版本更新说明**：右上角语言下拉分别填 `简体中文` 和 `English`。
3. **应用信息**：按语言分别填。
   - **应用名称、应用简介由 LPK 带出并锁定**，改不了，也不用改。
   - 关键词、应用描述、来源 URL、来源作者可以填。
   - 点语言下拉里的 **添加语言** 加 `English`；**每个语言是独立条目**，
     中文填过的不会自动带到英文。
4. **应用截图：每个语言都要单独勾选「适用于桌面端/移动端」并单独上传**。
   截图和平台声明不是全应用共享的，英文条目默认是空的（这一步最容易漏）。
   - 桌面端至少 2 张，移动端至少 3 张；本项目 4+4 张都够。
5. 滚到页面底部点 **提交审核**。它会按语言逐条校验：名称、简介、平台声明、
   截图数量；缺哪个语言会直接提示。

提交成功后「审核列表」里会出现一条「等待审核」记录；被拒时「原因」列会写
原因，改完重新提交即可。

### 为什么没有手机安装包

LPK 是唯一的交付物，装在微服上；手机端是「懒猫微服」客户端用 WebView 打开
微服上的这个应用。商店里的「支持移动端」只是声明界面适配，决定商店展示和
手机截图要求，不参与打包。审核列表里的「下载APK / 手动生成APK」是平台按版本
自动生成的审核测试壳，不是开发者要上传的东西。

## 第 4 步（可选）：先走内测

想先让自己或几个测试账号装一遍，用内测通道，不经过公开审核：

```bash
~/.local/share/lzctools/node_modules/.bin/lzc-cli appstore pre-publish \
  lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.39.lpk \
  -c "首个版本，验证安装、播放和歌词"
```

没有 `-G` 时会让你选内测组；内测组在开发者中心/内测工具里创建。内测通过后，
再走第 5 步提交公开审核。

## 第 5 步：用 CLI 提交商店审核（可选）

网页流程（第 3 步）和 CLI 二选一即可；已经用网页提交过就不用再跑。

```bash
~/.local/share/lzctools/node_modules/.bin/lzc-cli appstore publish \
  lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.39.lpk \
  --clangs=zh:lazycat/store/changelog.zh-CN.txt \
  --clangs=en:lazycat/store/changelog.en-US.txt
```

- `--clangs` 的语言键是 `zh`、`en`（CLI 会把 locale 归一成语言），不是 `zh-CN`。
- 如果第 2 步还没在网页创建应用，CLI 会检测到并提示你用安装包信息创建；
  但它只会问名称/包名/来源/作者，**截图和图标还是要去网页补**，所以建议先做第 2 步。
- 命令成功后会显示「正在提交审核」/「应用提交成功」。之后在开发者中心能看到
  审核状态；审核通过后商店里就能搜到。
- 更新已有应用时 CLI 只要 LPK + 更新说明，**不用重传截图和资料**；输出里的
  `node-fetch` `DeprecationWarning` 可忽略，末尾出现
  `The application was submitted successfully` 就是成功（2026-11-11 用
  0.1.37 实测）。

## 多用户隔离（2026-11-12，0.1.37 被拒后）

- **拒因**：添加音乐文件夹时能看到微服不同账号之间的数据——选择文件夹页面在
  `home` 下同时列出了 `admin`、`test`；平台要求改为多实例，并暂时下架了应用。
- **根因**：`lzc-manifest.yml` 里 `multi_instance: false`。单实例下
  `/lzcapp/run/mnt/home` 是**全局用户文稿根**，直接拿它当音乐根目录浏览就会
  暴露所有人的目录。
- **修法**（0.1.39）：`multi_instance: true`——每个用户独立容器，
  `/lzcapp/run/mnt/home` 只映射本人文稿，`/lzcapp/var` 也各自独立。服务端在
  `LAZYCAT_APP_DEPLOY_UID` 非空时只认 `/lzcapp/run/mnt/home`，并修掉「目录为空
  就换下一个挂载点」的探测逻辑，避免退回箱子级路径。
- **注意**：单实例 ↔ 多实例切换会改变 `/lzcapp/var` 的存储路径，旧数据不会自动
  迁移；官方审核指南要求变更实例时自己做好迁移/恢复。0.1.37 刚上架即下架，
  用户数据影响很小，但升级前要跟用户说明。
- **提交记录**：0.1.39（多实例隔离 + 移除文件夹后列表立即更新）于 2026-11-12
  用 CLI 提交审核，输出 `The application was submitted successfully`。

## 第 6 步：审核关注点（对照自查）

| 审核项 | MyGo 的情况 |
| --- | --- |
| 资料完备、多语言 | `package.yml` 有 zh-CN / en-US；商店资料按第 2 步补齐 |
| 可安装、可加载 | 0.1.36 已在本机侧载验证；健康检查 `/api/health` |
| 稳定、启动快 | 启动即出页面；扫描在后台进行 |
| 响应 <5 分钟 | 网盘扫描大目录可能久，但页面不阻塞，可边扫边用 |
| 无打扰式更新提示 | Web 版关闭了自更新提示 |
| 真实用途 | 音乐播放器；网盘文件类型关联不是必需（非工具类） |
| 数据持久化 | `/lzcapp/var/mygomusic`，重启/升级保留 |
| 免密登录 | 应用没有登录页，身份由网关提供 |

可能被问到的两点，提前在审核备注/描述里说清楚：

1. **首次打开是空状态**，需要用户选一个网盘文件夹。这是设计使然，不是故障；
   空状态页面有明确按钮。
2. **在线歌词需要外网**（`net.internet`）。没有外网时本地 `.lrc` 和播放不受影响。

## 提交前 checklist

- [ ] `lazycat/package.yml` 版本已递增，`cmd/server/main.go` 与 `lazycat/README.md` 对齐
- [ ] `./scripts/build-lpk.sh` 成功，二进制是 `statically linked`
- [ ] `lzc-cli project lint` 无 error
- [ ] 本机侧载 0.1.39 能打开、能播放、重启后数据还在
- [ ] 开发者中心：图标 + 中英文各自的描述/关键词/来源
- [ ] **每个语言都勾选适用平台并上传截图**（桌面 ≥2、手机 ≥3）
- [ ] 网页流程：上传新版本 → 填更新说明 → 提交审核
- [ ] 或 CLI 流程：`lzc-cli appstore login` 已完成，提交前和用户确认

## 后续版本

改 `package.yml` 的 `version` → 重新构建 → 本机侧载验证 → `appstore publish`，
更新日志换成新版本的内容。商店版本和侧载版本共用一个包 ID，不要并行维护两份。
