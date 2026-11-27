# MyGo 音乐 for 懒猫微服 (LPK)

把 MyGo Music 打包成懒猫微服的 `.lpk`，侧载安装，自用。

> 手机/Android 客户端适配的通用默认检查清单（新老产品通用）见
> `~/.claude/lazycat-microserver.md` 第 3 节「移动端适配默认清单」；本文只记
> MyGo 的具体实现和排障。

## 文件

| 文件 | 作用 |
| --- | --- |
| `package.yml` | 包 ID、版本、名称、多语言描述（LPK v2 的静态元数据） |
| `lzc-manifest.yml` | 服务、路由、数据目录、环境变量、健康检查、网盘挂载 |
| `lzc-build.yml` | 打包配置；`contentdir: ./content` 把二进制和前端打进包 |
| `icon.png` | 512×512 应用图标 |
| `content/` | 构建产物（不入库）：服务端二进制、WebAssembly 前端、CA 证书 |

## 构建

不需要 Docker，也不需要连云端的构建机：

```bash
./scripts/build-lpk.sh
```

脚本会：编译 `linux/amd64` 的静态服务端、编译 WebAssembly 前端、把两者放进
`lazycat/content/`，再调用 `lzc-cli project build`。产物在
`lazycat/dist/cloud.lazycat.app.mygomusic-v<版本号>.lpk`（版本号见
`package.yml`）。打包完成后，如果本机有
`/lzcapp/document/`，脚本会直接把新包复制过去，并把网盘根目录里旧的
`cloud.lazycat.app.mygomusic-v*.lpk` 移到 `.Trash/`（可以用 `NETDISK=...` 改路径）。

`lzc-cli` 的查找顺序是 `$LZC_CLI`、`lzc-cli`、`~/.local/share/lzctools/...`，
最后才用 `npx @lazycatcloud/lzc-cli`。

版本号有两处，必须一起改：`lazycat/package.yml` 的 `version`（商店和安装包），
和 `internal/version/version.go` 的 `const Version`（设置页「音乐来源」旁显示、
`/api/status` 和 `-version` 输出）。`build-lpk.sh` 会先比对两者，不一致直接
失败，避免装上了却在设置里看到旧版本号。

## 安装

`build-lpk.sh` 已经把包放进懒猫网盘（在微服里就是 `/lzcapp/document/`），在
懒猫客户端的网盘里点它，选择安装。如果被拦截，先在客户端设置里允许安装非商店
应用。

也可以让 `lzc-cli` 直接装到已配置的微服：

```bash
lzc-cli lpk install lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.48.lpk
```

（需要先 `lzc-cli box add-by-ssh <用户> <地址>` 并 `lzc-cli box switch` 选中它。）

网盘根目录只保留最新的安装包，旧版本在 `.Trash/` 里，可以直接在客户端里点开安装。

注意：往网盘放 LPK 只升级**微服上的应用本体**（页面、wasm、服务端），不会改变
懒猫商店里的版本，也不会给手机新增/刷新「下载」入口。手机上已经装过的应用，
打开时加载的就是微服上的内容，所以侧载后把应用完全退出再打开即可看到新版本；
手机上还没有入口时，要走 `appstore pre-publish` 内测或正式发布（见 `STORE.md`）。

## 第一次使用

1. 打开应用，点左下角的「设置」。
2. 进入「音乐来源」，点「选择文件夹」。
3. 在打开的页面里浏览懒猫网盘，进到放音乐的文件夹，点「添加此文件夹」。
4. 返回播放器。扫描完成后，歌曲会出现在首页、歌曲、专辑和艺人页面里。

歌单、喜欢的歌和扫描缓存存在 `/lzcapp/var/mygomusic`（容器里的 `/data`），
重启和升级都不会丢。网盘是只读挂载的，播放器不会改动或复制你的音乐文件。

## 歌词

- 音频文件旁边有 `.lrc`（同名）时优先用它。
- 没有歌词的歌，播放时会按标题、艺人、专辑和时长到 LRCLib 查找，找到的
  同步歌词会缓存 90 天，没找到的记 7 天，避免重复请求。
- 需要微服能访问外网；关掉外网就不找在线歌词。

## 关键设计

- **不构建镜像**：`lzc-manifest.yml` 的 service 用官方运行时
  `registry.lazycat.cloud/lzc/lzcapp:3.20.3`；`contentdir` 里的
  `music-server` 直接作为 `entrypoint` 运行，位置是
  `/lzcapp/pkg/content/music-server`。这样构建只需要 Go、gcc 和 `lzc-cli`，
  不需要 Docker，也不需要把镜像推到任何仓库。
- **静态链接**：MyGo 的音频包会带进 purego，普通的 Go 构建会依赖 glibc 的
  动态加载器，而运行时是 musl 的 Alpine。脚本用
  `-linkmode external -extldflags "-static"` 链接成不依赖解释器的静态二进制，
  并在打包前检查 `file` 的结果，链接不静态就直接失败。
- **用 `services` + `routes`**：路由指向
  `music.cloud.lazycat.app.mygomusic.lzcapp:8090`；没有用 `application.image`，
  避免平台不执行镜像入口的问题。
- **网盘**：懒猫网盘是用户的文稿目录，用
  `ext_config.enable_document_access: true` 挂到
  `/lzcapp/run/mnt/home`（新版系统同时有 `/lzcapp/document`），并在
  `package.yml` 声明 `document.read`。服务端启动时依次探测
  `/lzcapp/run/mnt/home`、`/lzcapp/document`、`/lzcapp/media/RemoteFS`，
  选第一个可读的目录。`enable_media_access`、`media.read` 保留作兜底，
  `net.internet` 用于 LRCLib。权限没给时页面会明确提示读不到网盘。
- **多用户隔离**：`multi_instance: true`，微服上每个用户各有一个容器：
  `/lzcapp/run/mnt/home` 只对应本人的文稿目录，歌单、喜欢和扫描缓存也在各自的
  `/lzcapp/var` 里。单实例下这个路径是**全局**文稿根，浏览文件夹时会看到
  `admin`、`test` 等其他账号的目录，商店审核会直接拒绝（0.1.37 被拒过一次）。
  服务端还有一道兜底：`LAZYCAT_APP_DEPLOY_UID` 非空（多实例）时只使用
  `/lzcapp/run/mnt/home`，不再探测 `/lzcapp/document`、`/lzcapp/media` 等箱子级
  路径；探测挂载点时也不再因为目录为空就跳到下一个。
- **持久化**：`/lzcapp/var` 是平台保证保留的目录，映射到容器的 `/data`。
- **HTTPS 证书**：包里带了 `ca-certificates.crt`，用 `SSL_CERT_FILE` 指过去，
  默认运行时没有根证书也能访问 LRCLib。
- **音频**：桌面端通过带签名的 `/api/audio/<token>` 地址以 HTTP Range 流式播放，
  浏览器可以拖动进度；手机上不取 token，直接用 `/api/track/<id>`（见下一条）。
- **手机播放：交给媒体元素（0.1.45）**：手机上（`navigator.userAgent` 含
  `android`，或页面里有客户端注入的 `window.lzc_media_session`）不再由 Go 下载、
  解码，而是把歌交给页面自己的 `<audio>` 元素：`GET /api/track/<id>` 同源地址，
  浏览器自己取流、解码、出声，系统按媒体会话保活，息屏和锁屏都继续放；换歌只是给
  同一个元素换 `src`，不发 JSON-RPC，所以后台被掐网也不会打断换歌。这和懒猫音乐
  （`cloud.lazycat.app.lzcmusicplayer` 1.0.6）的机制一致：它的 LPK 里就是一个
  `new Audio()` + `/api/music/stream?path=…`，没有任何看门狗或重试。代码在
  `internal/player/sink.go`（sink 接口）、`internal/player/element_js.go`
  （js/wasm，Android 才启用）、`internal/websource/websource.go`
  （`StreamURL`）和 `internal/server/media.go`（`track`）。
  桌面浏览器和桌面应用仍走原来的解码路径，频谱、下载进度和响度均衡都不受影响。
  媒体元素模式下没有 ReplayGain（Gain/Peak 在 `Stream` RPC 的结果里，这里不取），
  音量仍按平方映射到元素音量，听感和原来一致。
- **手机**：窗口窄于 760px 时侧边栏收进左上角菜单，播放条变紧凑，页面变单列。
  手机上图标按钮的点击范围向外多留 5px（布局不变），手指更好按。
- **手机后台播放与锁屏控制**：懒猫应用走 HTTP，不是安全上下文，
  `navigator.mediaSession` 不存在；Android 客户端注入的是非标准的
  `window.lzc_media_session`（`setMetadata`/`setPositionState` 收 JSON 字符串，
  `setActionHandler` 只收动作名，动作从 `lzc_media_session_event` 事件回来）。
  `web/lzc-media.js` 把它适配成标准 MediaSession，并在 `wasm_exec.js` 之前加载，
  锁屏和通知栏就有播放/暂停/上一曲/下一曲/进度拖动。`scripts/build-lpk.sh`
  会把该文件一起拷进 `content/web/`，新增前端文件时记得加进那行 `cp`。
- **锁屏控制的调用方式与 seek 数据（0.1.47）**：客户端除了发
  `lzc_media_session_event`，锁屏和通知栏的按钮还会直接调用页面上的
  `window.__nativePlay/__nativePause/__nativePrev/__nativeNext/__nativeSeekTo`
  （懒猫音乐 1.0.6 的 `mediaSessionBridge.ts` 用法相同），适配器两路都接。
  seek 的时间单位是**秒**，但客户端偶尔发空 `data`、字符串，或把毫秒当秒发；
  适配器 `seekTime` 接受数字/数字字符串/`{seekTime}`/`{position}` 等形状，
  超出会话时长且除以 1000 后落在时长内的才按毫秒读，没有有效数字的 seek 直接
  丢弃。以前 `data` 缺失时 vendored 的
  `vendor/github.com/egoist/mygo/internal/web/nowplaying.go` 直接
  `Value.Float()`，会 panic 并让整个 Go wasm 退出（页面和 `<audio>` 还在放），
  表现为拖一下进度条后所有按钮失效、重新打开应用显示「没有播放」。该文件现在
  先判类型再取值，并包一层 `recover`；升级 MyGo 重新 vendor 后要重新应用。
- **手机上的 seek 立即跳（0.1.47）**：媒体元素模式下 `internal/player/player.go`
  的 `Seek` 走 `instantSeeker`（`internal/player/element_js.go` 的
  `SeekInstant`），直接设 `currentTime`，不再暂停等待解码、不走计时器：锁屏时
  页面在后台，计时器可能被拖延，seek 要立即生效。元素没有 `src`
  （`networkState == 0`）时不设 `currentTime`，否则会抛异常结束程序。
- **解码路径的息屏继续播放（vendored 补丁，桌面浏览器）**：浏览器音频共用页面的
  `AudioContext`，MyGo 的输出空闲 5 秒会关闭它（suspend）；页面在后台时，被挂起的
  context 可能没有用户手势就 resume 不了，上一首放完后下一首没声，中间还有约 1 秒
  断流杂音。`vendor/github.com/egoist/mygo/audio/output.go` 加
  `keepOpenIdle`：`document.hidden` 时空闲也不关输出；
  `output_js.go` 在 `visibilitychange` 时再尝试 resume。升级 MyGo 重新
  `go mod vendor` 后要重新应用这两处。手机上 0.1.45 起不再用这条路径。
- **加载失败先重试（0.1.43–0.1.44，媒体元素模式同样有效）**：手机在后台时，
  懒猫客户端可能被系统掐掉网络（官方文档要求给客户端开「允许后台流量」
  「允许后台运行」）。以前加载失败 `loadFailed` 会立刻跳下一首，表现就是无声地
  连切、一直「正在缓冲」。现在 `internal/player/player.go` 的 `retryFailed`
  先原地重试：退避 1/2/4/8/15/30 秒，`Hidden`（`mygo.App.IsHidden`，即
  `document.hidden`）为真时一直等，在前台最多 6 次后才跳过；
  `visibilitychange` 回到前台立即 `RetryNow`。媒体元素报错也走这里（浏览器
  分不清「取不到」和「解不了」，都先重试）。解码下载层
  `internal/player/stream.go` 的 15 秒看门狗从**发起请求**就开始计时，
  不只是等数据：连接还在但服务器不响应时也会超时失败并自动重试；已有数据时带
  `Range` 从断点续传（最多 3 次）。重试期间播放条显示「加载失败，正在重试…」，
  和正常的「正在缓冲…」区分开（0.1.44）。
- **触屏滚动（vendored 补丁）**：上游 MyGo 的 web 后端只认鼠标，触摸拖动不会
  产生滚动事件，加上画布 `touch-action: none`，手机上内容滚不动。本项目用
  `go mod vendor` 把 MyGo 放进 `vendor/`，新增
  `vendor/github.com/egoist/mygo/internal/web/touch.go`，在 `surface.go`
  里接上指针事件：点按照常派发（按钮、滑块可用），移动超过 8px 后纵向拖动
  转为 `PointerScroll`（先 `PointerCancel` 防止误点），横向拖动保留按压给
  进度条并横向滚动，松手后带惯性。升级 MyGo 时先保留这个文件和
  `surface.go` 里的改动；如果重新 `go mod vendor`，改完要重新应用。

## 本地开发

不用微服也能跑：

```bash
./scripts/build-web.sh
go run ./cmd/server -music /path/to/music -data ./data
# 打开 http://localhost:8090/setup 选择文件夹
```

## 排障

- **安装后打不开**：看应用日志。多半是 `entrypoint` 路径或 `MUSIC_DIR` 不对。
- **看不到网盘文件夹**：确认包声明了 `document.read`、manifest 里有
  `enable_document_access: true`，并在客户端里允许应用读取文稿/网盘；
  装完重启一次应用。页面显示「网盘目录」就是服务端实际读的路径。
- **没有在线歌词**：微服访问不了 `lrclib.net`，或者包里的 CA 证书没生效；
  检查 `SSL_CERT_FILE` 和 `/lzcapp/pkg/content/ca-certificates.crt`。
- **升级**：改 `package.yml` 的 `version` 后重新构建、重新安装；
  数据在 `/lzcapp/var/mygomusic`，不受影响。
- **息屏后不接着放 / 换歌没声音 / 一直「正在缓冲」**：0.1.45 起手机上由页面的
  媒体元素自己取流播放，换歌不发请求，正常情况下不再出现。确认加载的
  `app.wasm` 是 0.1.45 起的（手机上打开应用看设置页的版本号；页面由微服提供，
  侧载新 LPK 后要完全退出应用再打开）。确认方式是 `navigator.userAgent` 含
  `android` 或页面有 `window.lzc_media_session`：满足时走媒体元素，否则仍走
  解码路径。仍出现时看播放条显示「正在缓冲…」还是「加载失败，正在重试…」：
  后者说明元素报了错而在自动重试，多半是网络，给它开后台流量/后台运行。
- **锁屏/通知栏没有播放控制**：确认 `web/lzc-media.js` 在 `content.tar` 的
  `web/` 里、`index.html` 在 `wasm_exec.js` 之前引用了它（0.1.42 起，仅
  Android 客户端；桌面浏览器用标准 MediaSession）。0.1.47 起还要求页面定义
  `window.__native*`，否则息屏后的按钮不触发；看控制台有没有
  `media session handler` 的报错。
- **拖进度条后上一首/下一首/暂停全失效、歌还在放、重开应用显示没在播放**：
  客户端发来的 seek 没有数字时间，Go wasm panic 退出而 `<audio>` 继续放。
  0.1.47 已修（适配器丢弃无效 seek，vendored `nowplaying.go` 判类型并 recover）。
  先确认设置页版本号是 0.1.47 起；仍复现时在页面远程调试里看
  `lzc_media_session_event` 的 `detail`，把形状补进 `seekTime`。
