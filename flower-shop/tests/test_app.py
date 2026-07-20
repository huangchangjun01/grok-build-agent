import os
import tempfile
import unittest

from app import create_app


class FlowerShopTests(unittest.TestCase):
    def setUp(self):
        self.db_fd, db_path = tempfile.mkstemp(suffix=".db")
        os.close(self.db_fd)
        self.app = create_app(
            {
                "TESTING": True,
                "DATABASE": db_path,
                "SECRET_KEY": "test-secret",
                "CSRF_ENABLED": False,
            }
        )
        self.client = self.app.test_client()

    def tearDown(self):
        os.unlink(self.app.config["DATABASE"])

    def _get_csrf_token(self):
        with self.client.session_transaction() as session:
            return session.get("csrf_token", "")

    def _post(self, url, data):
        token = self._get_csrf_token()
        payload = {"csrf_token": token, **data}
        return self.client.post(url, data=payload, follow_redirects=True)

    def test_index_loads(self):
        response = self.client.get("/")
        self.assertEqual(response.status_code, 200)
        self.assertIn("花语小店".encode(), response.data)

    def test_flower_lifecycle(self):
        response = self._post(
            "/flowers/add",
            {"name": "红玫瑰", "category": "玫瑰", "price": "9.90", "stock": "12"},
        )
        self.assertEqual(response.status_code, 200)
        self.assertIn("添加成功".encode(), response.data)

        list_response = self.client.get("/flowers")
        self.assertIn("红玫瑰".encode(), list_response.data)

        flower_id = self._latest_flower_id()
        self.assertIsNotNone(flower_id)

        delete_response = self._post(f"/flowers/{flower_id}/delete", {})
        self.assertEqual(delete_response.status_code, 200)

        list_after = self.client.get("/flowers")
        self.assertNotIn("红玫瑰".encode(), list_after.data)

    def test_customer_add_validates_email(self):
        response = self._post(
            "/customers/add",
            {"name": "李华", "phone": "13800000000", "email": "not-an-email"},
        )
        self.assertIn("邮箱".encode(), response.data)

        response_ok = self._post(
            "/customers/add",
            {"name": "李华", "phone": "13800000000", "email": "lihua@example.com"},
        )
        self.assertIn("添加成功".encode(), response_ok.data)

    def test_order_creation_and_stock_deduction(self):
        self._post(
            "/flowers/add",
            {"name": "白百合", "category": "百合", "price": "12.50", "stock": "10"},
        )
        self._post(
            "/customers/add",
            {"name": "王芳", "phone": "13900000000", "email": "wf@example.com"},
        )

        flower_id = self._latest_flower_id()
        customer_id = self._latest_customer_id()
        self.assertIsNotNone(flower_id)
        self.assertIsNotNone(customer_id)

        order_response = self._post(
            "/orders/add",
            {
                "customer_id": str(customer_id),
                "flower_id": str(flower_id),
                "quantity": "3",
            },
        )
        self.assertIn("订单创建成功".encode(), order_response.data)

        with self.app.app_context():
            from app import get_db

            db = get_db()
            stock = db.execute(
                "SELECT stock FROM flowers WHERE id = ?", (flower_id,)
            ).fetchone()["stock"]
            self.assertEqual(stock, 7)
            order_count = db.execute("SELECT COUNT(*) FROM orders").fetchone()[0]
            self.assertEqual(order_count, 1)

    def test_order_rejects_insufficient_stock(self):
        self._post(
            "/flowers/add",
            {"name": "向日葵", "category": "野花", "price": "5", "stock": "2"},
        )
        self._post(
            "/customers/add",
            {"name": "测试人", "phone": "13000000000", "email": ""},
        )

        flower_id = self._latest_flower_id()
        customer_id = self._latest_customer_id()

        response = self._post(
            "/orders/add",
            {
                "customer_id": str(customer_id),
                "flower_id": str(flower_id),
                "quantity": "5",
            },
        )
        self.assertIn("库存不足".encode(), response.data)

        with self.app.app_context():
            from app import get_db

            db = get_db()
            stock = db.execute(
                "SELECT stock FROM flowers WHERE id = ?", (flower_id,)
            ).fetchone()["stock"]
            self.assertEqual(stock, 2)
            self.assertEqual(db.execute("SELECT COUNT(*) FROM orders").fetchone()[0], 0)

    def _latest_flower_id(self):
        with self.app.app_context():
            from app import get_db

            return get_db().execute(
                "SELECT id FROM flowers ORDER BY id DESC LIMIT 1"
            ).fetchone()["id"]

    def _latest_customer_id(self):
        with self.app.app_context():
            from app import get_db

            return get_db().execute(
                "SELECT id FROM customers ORDER BY id DESC LIMIT 1"
            ).fetchone()["id"]


if __name__ == "__main__":
    unittest.main()