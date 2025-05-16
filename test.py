from flask import Flask, request

app = Flask(__name__)


@app.route('/webhook', methods=['POST'])
def micromdm_webhook():
    # MicroMDM 會把 event 以 JSON 傳過來
    data = request.json

    # 印出所有 event（你可以改成存到資料庫、寫檔案...）
    print("收到 MicroMDM Webhook event：")
    print(data)

    # 這邊可以根據 event 內容做判斷處理
    # 例如：if data.get("topic") == "mdm.Connect": ...

    return '', 200  # 只要回 200 表示收到


if __name__ == '__main__':
    app.run(host="0.0.0.0", port=5001)
