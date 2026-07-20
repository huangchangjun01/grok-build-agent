# 花语小店 · 简易花店管理系统

一个使用 **Flask + SQLite** 构建的小型花店管理示例，覆盖鲜花库存、订单与客户的基本业务。

## 功能特性

- 🛒 **鲜花库存管理**：添加、查看、删除鲜花，自动展示库存预警
- 🧾 **订单管理**：从现有客户与鲜花中下单，自动校验库存并扣减
- 👥 **客户管理**：登记客户姓名、电话、邮箱并查看列表
- 🛡️ **基础安全**：内置 CSRF 令牌、表单校验、参数化 SQL、防 XSS 模板渲染
- 💾 **零配置存储**：使用本地 SQLite 数据库（`flower_shop.db`），首次启动自动建表

## 目录结构

```
flower-shop/
├── app.py              # Flask 应用入口与路由
├── schema.sql          # 数据库表结构
├── requirements.txt    # Python 依赖
├── README.md           # 当前文档
├── static/
│   └── style.css       # 页面样式
├── templates/
│   ├── base.html       # 公共布局
│   ├── index.html      # 首页（统计概览）
│   ├── flowers/        # 鲜花相关页面
│   ├── customers/      # 客户相关页面
│   └── orders/         # 订单相关页面
└── tests/
    └── test_app.py     # 关键路径的单测
```

## 快速开始

```bash
cd /workspace/flower-shop
python3 -m venv .venv
source .venv/bin/activate            # Windows: .venv\Scripts\activate
pip install -r requirements.txt

# 初始化数据库（首次运行 app.py 时会自动建表，此命令可手动触发）
flask --app app.py init-db

# 启动开发服务器
python app.py
# 或：flask --app app.py run --debug
```

打开浏览器访问 <http://127.0.0.1:5000/> 即可。

## 业务流程建议

1. **上架鲜花**：进入「鲜花库存」→「添加鲜花」
2. **登记客户**：进入「客户管理」→「添加客户」
3. **创建订单**：进入「订单管理」→「创建订单」，选择客户与鲜花并提交
4. **删除鲜花**：在库存列表点击「删除」，会一并级联保护历史订单（订单中的鲜花名称是快照）

## 数据模型

| 表名 | 主要字段 | 说明 |
| --- | --- | --- |
| `flowers` | `name`, `category`, `price`, `stock` | 鲜花库存 |
| `customers` | `name`, `phone`, `email` | 客户信息 |
| `orders` | `customer_id`, `flower_id`, `flower_name`, `quantity`, `unit_price`, `total_amount`, `status` | 订单记录 |

订单表里冗余存储了 `flower_name` / `unit_price`，即便后续删除鲜花，历史订单仍能正确展示。

## 运行测试

```bash
python -m unittest discover tests
```

测试用例覆盖：
- 鲜花增删
- 客户新增与重复防护
- 订单创建、库存扣减
- 库存不足时的事务回滚
- CSRF 校验

## 生产环境建议

- 设置环境变量 `SECRET_KEY` 为高熵随机值
- 使用 `gunicorn` 等 WSGI 服务器并配合反向代理
- 替换 SQLite 为生产级数据库（PostgreSQL/MySQL），迁移 ORM
- 启用 HTTPS、限流、登录认证等