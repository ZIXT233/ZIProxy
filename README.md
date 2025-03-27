# ZIProxy
## 简介
ZIProxy是一款集成代理系统，支持可插拔协议实现，路由管理，多用户管理，自带web管理前端，使用vue3实现。
## 编译
`go build`
会生成ziproxy可执行文件。
## 使用
`./ziproxy`
默认会加载当前目录下的config.json配置文件。
`./ziproxy -c example.json`
参数指定配置文件

ziproxy运行需要依赖一些静态文件，请放置在当前目录static文件夹或者在配置中指定路径。
## 配置
`config.json`有如下配置项
```json5
{
  //代理配置db路径，使用sqlite3
  "db": "ziproxy.db",          

  //代理统计db路径，使用sqlite3 
  "statistic_db": "ziproxy_statistic.db",

  //web管理界面监听地址，127.0.0.1仅允许本机访问
  //任意地址访问请使用0.0.0.0
  //推荐使用https入站代理guestForward跳转的方式外部访问管理界面
  "web_address": "127.0.0.1:2339",

  //web后端生成jwt时所用的密钥
  "web_secret": "23333",

  //静态文件夹路径
  "static_path": "./static"
}
```

第一次启动ziproxy时，会生成示例代理配置数据表，请在管理面板中查看。
面板管理员默认用户名`admin`密码`admin`
请不要删除"管理员"用户组,否则之后无法登录管理面板。
“游客”用户组支持无验证连接代理。

## http代理guestForward跳转
当http头部不携带用户名密码时，如果http入站代理指定了guestForward地址，会将流量反向代理到指定地址。可以将该地址指定为web面板访问地址实现代理信道访问面板。
