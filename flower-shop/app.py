import os
import secrets
import sqlite3
from decimal import Decimal, InvalidOperation
from pathlib import Path

from flask import (
    Flask,
    abort,
    current_app,
    flash,
    g,
    redirect,
    render_template,
    request,
    session,
    url_for,
)

BASE_DIR = Path(__file__).resolve().parent


def get_db():
    if "db" not in g:
        g.db = sqlite3.connect(current_app.config["DATABASE"])
        g.db.row_factory = sqlite3.Row
        g.db.execute("PRAGMA foreign_keys = ON")
    return g.db


def close_db(_error=None):
    db = g.pop("db", None)
    if db is not None:
        db.close()


def init_db():
    db = get_db()
    schema_path = BASE_DIR / "schema.sql"
    db.executescript(schema_path.read_text(encoding="utf-8"))
    db.commit()


def parse_positive_decimal(value, field_name):
    try:
        number = Decimal(value)
    except (InvalidOperation, TypeError):
        raise ValueError(f"{field_name}必须是有效数字") from None
    if not number.is_finite() or number < 0:
        raise ValueError(f"{field_name}不能小于 0")
    return number.quantize(Decimal("0.01"))


def parse_positive_int(value, field_name, allow_zero=False):
    try:
        number = int(value)
    except (TypeError, ValueError):
        raise ValueError(f"{field_name}必须是整数") from None
    minimum = 0 if allow_zero else 1
    if number < minimum:
        comparator = "不能小于 0" if allow_zero else "必须大于 0"
        raise ValueError(f"{field_name}{comparator}")
    return number


def create_app(test_config=None):
    app = Flask(__name__)
    app.config.from_mapping(
        SECRET_KEY=os.environ.get("SECRET_KEY", "dev-secret-key-change-in-production"),
        DATABASE=str(BASE_DIR / "flower_shop.db"),
        CSRF_ENABLED=True,
    )

    if test_config:
        app.config.update(test_config)

    app.teardown_appcontext(close_db)

    @app.cli.command("init-db")
    def init_db_command():
        init_db()
        print("数据库初始化完成。")

    @app.context_processor
    def inject_template_helpers():
        if "csrf_token" not in session:
            session["csrf_token"] = secrets.token_hex(16)

        def format_currency(value):
            return f"¥{float(value):,.2f}"

        return {
            "csrf_token": session["csrf_token"],
            "format_currency": format_currency,
        }

    @app.before_request
    def validate_csrf():
        if not app.config.get("CSRF_ENABLED", True) or request.method != "POST":
            return None
        submitted_token = request.form.get("csrf_token", "")
        expected_token = session.get("csrf_token", "")
        if not expected_token or not secrets.compare_digest(submitted_token, expected_token):
            abort(400, description="无效或已过期的请求令牌，请刷新页面后重试。")
        return None

    @app.route("/")
    def index():
        db = get_db()
        stats = {
            "flowers": db.execute("SELECT COUNT(*) FROM flowers").fetchone()[0],
            "stock": db.execute("SELECT COALESCE(SUM(stock), 0) FROM flowers").fetchone()[0],
            "customers": db.execute("SELECT COUNT(*) FROM customers").fetchone()[0],
            "orders": db.execute("SELECT COUNT(*) FROM orders").fetchone()[0],
        }
        low_stock = db.execute(
            "SELECT * FROM flowers WHERE stock <= 5 ORDER BY stock ASC, name ASC LIMIT 5"
        ).fetchall()
        recent_orders = db.execute(
            """
            SELECT o.*, c.name AS customer_name
            FROM orders AS o
            JOIN customers AS c ON c.id = o.customer_id
            ORDER BY o.id DESC
            LIMIT 5
            """
        ).fetchall()
        return render_template(
            "index.html", stats=stats, low_stock=low_stock, recent_orders=recent_orders
        )

    @app.route("/flowers")
    def flower_list():
        flowers = get_db().execute(
            "SELECT * FROM flowers ORDER BY id DESC"
        ).fetchall()
        return render_template("flowers/list.html", flowers=flowers)

    @app.route("/flowers/add", methods=("GET", "POST"))
    def flower_add():
        if request.method == "POST":
            name = request.form.get("name", "").strip()
            category = request.form.get("category", "").strip()
            if not name:
                flash("请输入鲜花名称。", "error")
            elif len(name) > 100 or len(category) > 100:
                flash("名称和分类不能超过 100 个字符。", "error")
            else:
                try:
                    price = parse_positive_decimal(request.form.get("price"), "单价")
                    stock = parse_positive_int(
                        request.form.get("stock"), "库存", allow_zero=True
                    )
                except ValueError as error:
                    flash(str(error), "error")
                else:
                    db = get_db()
                    db.execute(
                        "INSERT INTO flowers (name, category, price, stock) VALUES (?, ?, ?, ?)",
                        (name, category, float(price), stock),
                    )
                    db.commit()
                    flash(f"鲜花“{name}”添加成功。", "success")
                    return redirect(url_for("flower_list"))
        return render_template("flowers/add.html")

    @app.post("/flowers/<int:flower_id>/delete")
    def flower_delete(flower_id):
        db = get_db()
        flower = db.execute(
            "SELECT name FROM flowers WHERE id = ?", (flower_id,)
        ).fetchone()
        if flower is None:
            abort(404)
        db.execute("DELETE FROM flowers WHERE id = ?", (flower_id,))
        db.commit()
        flash(f"鲜花“{flower['name']}”已删除。", "success")
        return redirect(url_for("flower_list"))

    @app.route("/customers")
    def customer_list():
        customers = get_db().execute(
            "SELECT * FROM customers ORDER BY id DESC"
        ).fetchall()
        return render_template("customers/list.html", customers=customers)

    @app.route("/customers/add", methods=("GET", "POST"))
    def customer_add():
        if request.method == "POST":
            name = request.form.get("name", "").strip()
            phone = request.form.get("phone", "").strip()
            email = request.form.get("email", "").strip()
            if not name:
                flash("请输入客户姓名。", "error")
            elif not phone:
                flash("请输入联系电话。", "error")
            elif len(name) > 100 or len(phone) > 30 or len(email) > 120:
                flash("客户信息超过允许的字符长度。", "error")
            elif email and ("@" not in email or email.startswith("@") or email.endswith("@")):
                flash("请输入有效的邮箱地址。", "error")
            else:
                db = get_db()
                db.execute(
                    "INSERT INTO customers (name, phone, email) VALUES (?, ?, ?)",
                    (name, phone, email),
                )
                db.commit()
                flash(f"客户“{name}”添加成功。", "success")
                return redirect(url_for("customer_list"))
        return render_template("customers/add.html")

    @app.route("/orders")
    def order_list():
        orders = get_db().execute(
            """
            SELECT o.*, c.name AS customer_name, c.phone AS customer_phone
            FROM orders AS o
            JOIN customers AS c ON c.id = o.customer_id
            ORDER BY o.id DESC
            """
        ).fetchall()
        return render_template("orders/list.html", orders=orders)

    @app.route("/orders/add", methods=("GET", "POST"))
    def order_add():
        db = get_db()
        customers = db.execute(
            "SELECT id, name, phone FROM customers ORDER BY name ASC"
        ).fetchall()
        flowers = db.execute(
            "SELECT id, name, price, stock FROM flowers WHERE stock > 0 ORDER BY name ASC"
        ).fetchall()

        if request.method == "POST":
            try:
                customer_id = parse_positive_int(request.form.get("customer_id"), "客户")
                flower_id = parse_positive_int(request.form.get("flower_id"), "鲜花")
                quantity = parse_positive_int(request.form.get("quantity"), "购买数量")
            except ValueError as error:
                flash(str(error), "error")
            else:
                try:
                    db.execute("BEGIN IMMEDIATE")
                    customer = db.execute(
                        "SELECT id FROM customers WHERE id = ?", (customer_id,)
                    ).fetchone()
                    flower = db.execute(
                        "SELECT id, name, price, stock FROM flowers WHERE id = ?",
                        (flower_id,),
                    ).fetchone()
                    if customer is None:
                        raise ValueError("所选客户不存在，请重新选择。")
                    if flower is None:
                        raise ValueError("所选鲜花不存在，请重新选择。")
                    if flower["stock"] < quantity:
                        raise ValueError(
                            f"库存不足，{flower['name']} 当前仅剩 {flower['stock']} 件。"
                        )

                    total_amount = Decimal(str(flower["price"])) * quantity
                    db.execute(
                        "UPDATE flowers SET stock = stock - ? WHERE id = ?",
                        (quantity, flower_id),
                    )
                    db.execute(
                        """
                        INSERT INTO orders (
                            customer_id, flower_id, flower_name, quantity,
                            unit_price, total_amount, status
                        ) VALUES (?, ?, ?, ?, ?, ?, ?)
                        """,
                        (
                            customer_id,
                            flower_id,
                            flower["name"],
                            quantity,
                            flower["price"],
                            float(total_amount),
                            "已创建",
                        ),
                    )
                    db.commit()
                except ValueError as error:
                    db.rollback()
                    flash(str(error), "error")
                except sqlite3.DatabaseError:
                    db.rollback()
                    current_app.logger.exception("创建订单时数据库操作失败")
                    flash("订单创建失败，请稍后重试。", "error")
                else:
                    flash("订单创建成功，库存已自动扣减。", "success")
                    return redirect(url_for("order_list"))

        return render_template(
            "orders/add.html", customers=customers, flowers=flowers
        )

    with app.app_context():
        init_db()

    return app


app = create_app()


if __name__ == "__main__":
    app.run(debug=True)
