# 架构设计

## 代码模块

1. mq: 负责将worker完成的工作量发布到MQ
2. chainnode: 负责适配各种链，获取任务模版；只能依赖comment模块
3. comment: 公共模块，最底层，不能依赖其它模块
4. gostratum: 管理worker
5. poolstratum: pool主业务
6. prom: TODO
