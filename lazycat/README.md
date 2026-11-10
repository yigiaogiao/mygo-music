# MyGo 音乐 for 懒猫微服 (LPK)

把 MyGo Music 打包成懒猫微服的 `.lpk`，侧载安装，自用。

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
`lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.36.lpk`。

`lzc-cli` 的查找顺序是 `$LZC_CLI`、`lzc-cli`、`~/.local/share/lzctools/...`，
最后才用 `npx @lazycatcloud/lzc-cli`。

## 安装

把 `.lpk` 放进懒猫网盘（在微服里就是 `/lzcapp/document/`），然后在懒猫客户端
的网盘里点它，选择安装。如果被拦截，先在客户端设置里允许安装非商店应用。

也可以让 `lzc-cli` 直接装到已配置的微服：

```bash
lzc-cli lpk install lazycat/dist/cloud.lazycat.app.mygomusic-v0.1.36.lpk
```

（需要先 `lzc-cli box add-by-ssh <用户> <地址>` 并 `lzc-cli box switch` 选中它。）

安装包已经复制到网盘根目录一份，可以直接在客户端里点开安装。

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
- **持久化**：`/lzcapp/var` 是平台保证保留的目录，映射到容器的 `/data`。
- **HTTPS 证书**：包里带了 `ca-certificates.crt`，用 `SSL_CERT_FILE` 指过去，
  默认运行时没有根证书也能访问 LRCLib。
- **音频**：通过带签名的 `/api/audio/<token>` 地址以 HTTP Range 流式播放，
  浏览器可以拖动进度。
- **手机**：窗口窄于 760px 时侧边栏收进左上角菜单，播放条变紧凑，页面变单列。

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
