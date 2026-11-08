# 部署到懒猫微服

这个版本把 MyGo Music 拆成两半：浏览器里的界面（Go 编译成 WebAssembly），
和跑在微服里的服务端。服务端读取懒猫网盘里你选的文件夹，播放时把音频流给
浏览器，没有歌词的歌会到 [LRCLib](https://lrclib.net) 找同步歌词。

## 打包

需要一台装有 Docker 的电脑，以及懒猫的 `lzc-cli`。

```bash
lzc-cli project build
```

产物在 `dist/` 里，是一个 `.lpk` 文件。它会用仓库根目录的 `Dockerfile`
构建镜像（Go 交叉编译出服务端和 wasm），再按 `lzc-manifest.yml` 打包。

## 安装

在微服的「应用商店」里上传这个 `.lpk`，或者：

```bash
lzc-cli lpk install dist/*.lpk
```

安装后在桌面和手机上都能打开「MyGo 音乐」。

## 第一次使用

1. 打开应用，点左下角的「设置」。
2. 进入「音乐来源」，点「选择文件夹」。
3. 在打开的页面里浏览懒猫网盘，进到放音乐的文件夹，点「添加此文件夹」。
4. 返回播放器。扫描完成后，歌曲会出现在首页、歌曲、专辑和艺人页面里。

歌单、喜欢的歌和扫描缓存存在 `/lzcapp/var/data`，重启应用不会丢。网盘是
只读挂载的，播放器不会改动或复制你的音乐文件。

## 歌词

- 音频文件旁边有 `.lrc`（同名）时优先用它。
- 没有歌词的歌，播放时会按标题、艺人、专辑和时长到 LRCLib 查找，找到的
  同步歌词会缓存 90 天，没找到的记 7 天，避免重复请求。
- 需要应用有外网权限（`net.internet`）；关掉外网就不找在线歌词。

## 说明

- 懒猫网盘的挂载路径是 `/lzcapp/media/RemoteFS`，由 manifest 里的
  `ext_config.enable_media_access: true` 打开。
- 音频通过带签名的地址以 HTTP Range 流式播放，浏览器可以拖动进度。
- 手机上界面会自动切成单列：侧边栏收到左上角的菜单里，播放条变成紧凑的
  一条，点封面进入「正在播放」。

## 本地开发

不用微服也能跑：把音乐目录挂到本地路径即可。

```bash
./scripts/build-web.sh
go run ./cmd/server -music /path/to/music -data ./data
# 打开 http://localhost:8090/setup 选择文件夹
```
