import os
import csv
import base64
import requests
import subprocess
from dotenv import load_dotenv
from rich.table import Table
from rich.console import Console
from rich.prompt import Prompt, Confirm
import time
import threading

import json
import socketio
# 載入 .env 檔案
load_dotenv()

# 設定常數
VPPTOKEN_PATH = 'ISHA_APP_token.vpptoken'

API_KEY = os.getenv('API_KEY')
MDM_URL = os.getenv('MDM_URL')
WEBSOCKET_URL = os.getenv('WEBSOCKET_URL')
DEVICE_LIST_CSV = './devices.csv'
MDMCTL_BIN = 'mdmctl'
PROFILES_DIR = './profiles'

# 確保目錄存在
os.makedirs(PROFILES_DIR, exist_ok=True)

console = Console()

# 創建 Socket.IO 客戶端
sio = socketio.Client()


@sio.event
def connect():
    console.print("[SocketIO] 已連接到 webhook 伺服器!", style="bold green")
    sio.emit('auth', {'api_key': API_KEY})

@sio.on('auth_result')
def on_auth_result(data):
    print("認證回應:", data)
    if data['status'] == 'ok':
        print("Auth success! Now ready to receive events.")
    else:
        print("Auth failed!")

@sio.event
def disconnect():
    console.print("[SocketIO] 與 webhook 伺服器斷開連接", style="bold red")


@sio.on('mdm_event')
def on_mdm_event(data):
    # console.print("[SocketIO] 收到 MDM 事件：", style="bold green")
    # console.print(json.dumps(data, indent=2, ensure_ascii=False))

    # 處理不同類型的事件
    if 'acknowledge_event' in data:
        # console.print("[SocketIO] Acknowledge 事件：", style="bold blue")
        # console.print(json.dumps(data['acknowledge_event'], indent=2, ensure_ascii=False))

        # 如果有 raw_payload，嘗試解碼
        if 'raw_payload' in data['acknowledge_event']:
            try:
                raw = data['acknowledge_event']['raw_payload']
                decoded = base64.b64decode(raw).decode(errors='ignore')
                console.print("[SocketIO] 解碼的 raw_payload：", style="bold green")
                console.print(decoded)
            except Exception as e:
                console.print(f"[SocketIO] 解碼 raw_payload 錯誤：{str(e)}", style="bold red")

    elif 'checkin_event' in data:
        pass
        # console.print("[SocketIO] Checkin 事件：", style="bold blue")
        # console.print(json.dumps(data['checkin_event'], indent=2, ensure_ascii=False))

    elif data.get('type') == 'server_info':
        console.print(f"[SocketIO] 伺服器訊息: {data.get('message')}", style="bold cyan")

    else:
        console.print("[SocketIO] 其他 MDM 事件：", style="bold blue")
        console.print(json.dumps(data, indent=2, ensure_ascii=False))


def start_socketio_client():
    # 從環境變數或配置獲取 webhook 伺服器地址
    ws_host = os.getenv('WEBHOOK_HOST', WEBSOCKET_URL)
    ws_port = os.getenv('WEBHOOK_PORT', '443')
    socketio_url = f"https://{ws_host}:{ws_port}"

    console.print(f"[SocketIO] 正在連接到 webhook 伺服器 {socketio_url}", style="bold blue")

    def run_client():
        while True:
            try:
                if not sio.connected:
                    sio.connect(socketio_url)
                    console.print("[SocketIO] 連接成功", style="bold green")
                time.sleep(1)  # 定期檢查連接狀態
            except Exception as e:
                console.print(f"[SocketIO] 連接錯誤: {str(e)}", style="bold red")
                # 連接失敗，等待後重試
                time.sleep(5)

    # 在單獨的線程中啟動 Socket.IO 客戶端
    thread = threading.Thread(target=run_client, daemon=True)
    thread.start()
    return thread


def run_mdmctl_get_devices(output_file):
    console.print("📥 取得所有裝置資料...", style="bold blue")
    command = f"{MDMCTL_BIN} get devices"
    awk_cmd = "awk 'NR>1 && $1 != \"\" {print $1 \",\" $2}'"
    full_cmd = f"{command} | {awk_cmd}"
    with open(output_file, "w") as f:
        subprocess.run(full_cmd, shell=True, stdout=f)

def get_device_from_net(server_url, api_key, output_file):
    console.print("📥 取得所有裝置資料...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)

    resp = requests.post(
        f"{server_url}/v1/devices",
        headers=headers,
        auth=auth,
        data=json.dumps({})
    )

    if resp.status_code == 200:
        data = resp.json()
        with open(output_file, "w", newline='') as f:
            writer = csv.writer(f)
            for device in data.get("devices", []):
                udid = device.get("udid", "")
                serial = device.get("serial_number", "")
                writer.writerow([udid, serial])
        console.print("✅ 成功取得裝置資料", style="green")
    else:
        console.print(f"❌ 錯誤：{resp.status_code}", style="bold red")
        console.print(resp.text)

    return resp.status_code

def load_sToken(vpptoken_path):
    with open(vpptoken_path, 'r') as f:
        encoded = f.read().strip()
        return encoded


def assign_vpp_license(sToken, adamId, serialNumber):
    console.print(f"🔑 分配 VPP 授權給序號 {serialNumber}...", style="bold green")
    data = {
        "sToken": sToken,
        "adamIdStr": str(adamId),
        "associateSerialNumbers": [serialNumber]
    }
    resp = requests.post(
        "https://vpp.itunes.apple.com/mdm/manageVPPLicensesByAdamIdSrv",
        headers={"Content-Type": "application/json"},
        data=json.dumps(data)
    )
    console.print(f"✅ Apple 回應 ({serialNumber}):", resp.status_code, style="green")
    try:
        console.print(resp.json())
    except Exception:
        console.print(resp.text)


def install_app_to_device(server_url, api_key, udid, app_id):
    console.print(f"🚀 安裝 App 到 UDID={udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "InstallApplication",
        "itunes_store_id": int(app_id),
        "options": {
            "purchase_method": 1
        }
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ MicroMDM 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def install_enterprise_app(server_url, api_key, udid, manifest_url):
    console.print(f"🚀 安裝企業 App 到 UDID={udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "InstallEnterpriseApplication",
        "manifest_url": manifest_url
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ MicroMDM 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def lock_device(server_url, api_key, udid, pin=None, message=None):
    console.print(f"🔒 鎖定裝置 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceLock"
    }
    if pin:
        payload["pin"] = pin
    if message:
        payload["message"] = message

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 鎖定結果 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def restart_device(server_url, api_key, udid):
    console.print(f"🔄 重開機 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RestartDevice"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 重開機回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp


def shutdown_device(server_url, api_key, udid):
    """
    遠端關機裝置
    :param server_url: MicroMDM 伺服器網址（例如 https://mdm.example.com）
    :param api_key: 認證用的 API 金鑰
    :param udid: 裝置的 UDID
    :return: requests 回應物件
    """
    console.print(f"⏹️ 正在關機 {udid}...", style="bold red")

    headers = {
        "Content-Type": "application/json"
    }
    auth = ('micromdm', api_key)

    payload = {
        "udid": udid,
        "request_type": "ShutDownDevice"
    }

    resp = requests.post(
        f"{server_url}/v1/commands",
        headers=headers,
        auth=auth,
        data=json.dumps(payload)
    )

    console.print(f"🔌 關機回應 ({udid}): {resp.status_code}", style="green")
    console.print(resp.text)
    return resp

def clear_passcode(server_url, api_key, udid):
    console.print(f"🔓 清除密碼 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ClearPasscode"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 清除密碼回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def erase_device(server_url, api_key, udid, pin=None):
    console.print(f"💥 擦除裝置 {udid}...", style="bold red")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "EraseDevice"
    }
    if pin:
        payload["pin"] = pin

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 擦除回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def remove_application(server_url, api_key, udid, identifier="*"):
    console.print(f"🧹 移除應用程式 {identifier} 從 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RemoveApplication",
        "identifier": identifier
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_device_info(server_url, api_key, udid):
    console.print(f"📊 獲取裝置詳細資訊 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceInformation",
        "queries": [
            "UDID", "DeviceName", "OSVersion",
        ]
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_installed_apps(server_url, api_key, udid):
    console.print(f"📋 獲取已安裝應用程式清單 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "InstalledApplicationList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_profiles(server_url, api_key, udid):
    console.print(f"📋 獲取已安裝描述檔清單 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ProfileList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_os_updates(server_url, api_key, udid):
    console.print(f"🔍 查詢可用系統更新 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "AvailableOSUpdates"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def schedule_os_update(server_url, api_key, udid, product_key, product_version, install_action="InstallASAP"):
    console.print(f"📲 排程系統更新 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ScheduleOSUpdate",
        "updates": [
            {
                "install_action": install_action,
                "product_key": product_key,
                "product_version": product_version,
                "max_user_deferrals": 1,
                "priority": "High"
            }
        ],
        "command_uuid": f"update_{int(time.time())}"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def install_profile(server_url, api_key, udid, profile_path):
    console.print(f"📝 安裝描述檔到 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)

    # 讀取 profile 並進行 base64 編碼
    with open(profile_path, 'rb') as f:
        content = f.read()
        payload_base64 = base64.b64encode(content).decode('utf-8')

    payload = {
        "udid": udid,
        "request_type": "InstallProfile",
        "payload": payload_base64
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def remove_profile(server_url, api_key, udid, identifier):
    console.print(f"🗑️ 移除描述檔 {identifier} 從 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RemoveProfile",
        "identifier": identifier
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def setup_account(server_url, api_key, udid, fullname, username, lock_info=True):
    console.print(f"👤 設定裝置帳號 {username} 到 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "AccountConfiguration",
        "skip_primary_setup_account_creation": False,
        "set_primary_setup_account_as_regular_user": False,
        "dont_auto_populate_primary_account_info": False,
        "lock_primary_account_info": lock_info,
        "primary_account_full_name": fullname,
        "primary_account_user_name": username
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def device_configured(server_url, api_key, udid):
    console.print(f"✅ 標記裝置已配置完成 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceConfigured",
        "request_requires_network_tether": False
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_activation_lock_bypass(server_url, api_key, udid):
    console.print(f"🔑 獲取啟用鎖繞過碼 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ActivationLockBypassCode"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_security_info(server_url, api_key, udid):
    console.print(f"🔒 獲取安全資訊 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "SecurityInfo"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_certificate_list(server_url, api_key, udid):
    console.print(f"🔐 獲取憑證清單 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "CertificateList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def clear_command_queue(server_url, api_key, udid):
    console.print(f"🧹 清除命令佇列 {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.delete(f"{server_url}/v1/commands/{udid}", auth=auth)
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def inspect_command_queue(server_url, api_key, udid):
    console.print(f"🔍 檢查命令佇列 {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.get(f"{server_url}/v1/commands/{udid}", auth=auth)
    console.print(f"✅ 回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def push_device_with_mdmctl(udid):
    result = subprocess.run(["mdmctl", "push", udid], capture_output=True, text=True)
    if result.returncode == 0:
        console.print(f"✅ mdmctl push 成功 ({udid})", style="green")
    else:
        console.print(f"❌ mdmctl push 失敗 ({udid}):", style="red")
        console.print(result.stderr)


def send_push_to_device(server_url, api_key, udid):
    console.print(f"🔔 發送 Push 通知給裝置 {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    try:
        resp = requests.get(f"{server_url}/push/{udid}", auth=auth)
        console.print(resp.text)
        if resp.status_code == 200:
            console.print(f"✅ Push 通知回應 ({udid}): 200", style="green")
        else:
            console.print(f"❌ Push 失敗，嘗試改用 mdmctl push", style="bold yellow")
            push_device_with_mdmctl(udid)
    except Exception as e:
        console.print(f"⚠️ Push 發生錯誤：{str(e)}，改用 mdmctl push", style="bold yellow")
        push_device_with_mdmctl(udid)


def sync_dep_devices(server_url, api_key):
    console.print(f"🔄 同步 DEP 裝置...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.post(f"{server_url}/v1/dep/syncnow", auth=auth)
    console.print(f"✅ 回應: {resp.status_code}", style="green")
    console.print(resp.text)
    return resp.status_code


def parse_app_id(input_str):
    if input_str.startswith("http"):
        return input_str.split("id")[-1].split("?")[0]
    return input_str.strip()


def enable_lost_mode(server_url, api_key, udid, message=None, phone_number=None, footnote=None):
    """啟用遺失模式"""
    console.print(f"🔍 啟用遺失模式 {udid}...", style="bold red")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "EnableLostMode"
    }

    # 添加可選參數
    if message:
        payload["message"] = message
    if phone_number:
        payload["phone_number"] = phone_number
    if footnote:
        payload["footnote"] = footnote

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 遺失模式啟用回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def disable_lost_mode(server_url, api_key, udid):
    """關閉遺失模式"""
    console.print(f"🔓 關閉遺失模式 {udid}...", style="bold green")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DisableLostMode"
    }

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 遺失模式關閉回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_device_location(server_url, api_key, udid):
    """獲取設備位置（僅在遺失模式下可用）"""
    console.print(f"📍 獲取設備位置 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceLocation"
    }

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 設備定位回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)

    # 處理常見錯誤碼
    if resp.status_code == 200:
        try:
            response_data = resp.json()
            if 'error_code' in response_data:
                error_code = response_data['error_code']
                if error_code == 12067:
                    console.print(f"⚠️ 錯誤：設備 {udid} 未處於遺失模式", style="bold yellow")
                elif error_code == 12068:
                    console.print(f"⚠️ 錯誤：設備 {udid} 位置未知", style="bold yellow")
                elif error_code == 12078:
                    console.print(f"⚠️ 錯誤：設備 {udid} 在遺失模式下收到無效命令", style="bold yellow")
        except:
            pass

    return resp.status_code


def play_lost_mode_sound(server_url, api_key, udid):
    """播放遺失模式聲音（僅在遺失模式下可用）"""
    console.print(f"🔊 播放遺失模式聲音 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "PlayLostModeSound"
    }

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 播放聲音回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def wait_device_info(server_url, api_key, udid, max_retry=5, sleep_time=4):
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    data = json.dumps({})
    for i in range(max_retry):
        resp_info = requests.get(f"{server_url}/v1/devices/{udid}", headers=headers, auth=auth,data=data)
        if resp_info.status_code == 200:
            return resp_info.json()
        else:
            console.print(f"第{i+1}次查詢... 目前無資料，{resp_info.status_code}", style="yellow")
            time.sleep(sleep_time)
    return None


def check_lost_mode_status(server_url, api_key, udid):
    """檢查設備是否在遺失模式"""
    console.print(f"🔍 檢查遺失模式狀態 {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "SecurityInfo"
    }

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 安全資訊查詢回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_device_location_with_check(server_url, api_key, udid):
    """獲取設備位置（先檢查遺失模式狀態）"""
    console.print(f"📍 準備獲取設備位置 {udid}...", style="bold blue")

    # 先檢查設備狀態
    console.print("🔍 正在檢查設備是否處於遺失模式...", style="yellow")
    check_response = check_lost_mode_status(server_url, api_key, udid)

    if check_response == 201:
        console.print("✅ 狀態檢查命令已發送，請等待回應確認遺失模式狀態", style="green")
        time.sleep(2)  # 稍等一下讓設備回應

    # 無論如何都嘗試獲取位置
    console.print(f"📍 嘗試獲取設備位置...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceLocation"
    }

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ 設備定位回應 ({udid}):", resp.status_code, style="green")
    console.print(resp.text)

    return resp.status_code


# 增強的 SocketIO 事件處理 - 添加到現有的 on_mdm_event 函數中
def enhanced_on_mdm_event(data):
    """增強版 MDM 事件處理，專門處理位置和遺失模式回應"""

    if 'acknowledge_event' in data:
        ack_event = data['acknowledge_event']

        # 檢查是否是位置回應
        if 'command_type' in ack_event and ack_event['command_type'] == 'DeviceLocation':
            console.print("[位置回應] 收到設備位置資訊！", style="bold green")

            # 解析位置數據
            if 'status' in ack_event:
                if ack_event['status'] == 'Acknowledged':
                    console.print("✅ 設備已確認位置請求", style="green")
                elif ack_event['status'] == 'Error':
                    error_code = ack_event.get('error_code', 'Unknown')
                    if error_code == 12067:
                        console.print("❌ 錯誤：設備未處於遺失模式", style="bold red")
                    elif error_code == 12068:
                        console.print("❌ 錯誤：設備位置未知", style="bold red")
                    else:
                        console.print(f"❌ 錯誤代碼：{error_code}", style="bold red")

        # 檢查是否是遺失模式狀態回應
        elif 'command_type' in ack_event and ack_event['command_type'] == 'SecurityInfo':
            console.print("[安全資訊] 收到設備安全狀態！", style="bold blue")

        # 檢查是否是遺失模式啟用/關閉回應
        elif 'command_type' in ack_event and ack_event['command_type'] in ['EnableLostMode', 'DisableLostMode']:
            command_type = ack_event['command_type']
            if ack_event.get('status') == 'Acknowledged':
                if command_type == 'EnableLostMode':
                    console.print("✅ 遺失模式已成功啟用！", style="bold green")
                else:
                    console.print("✅ 遺失模式已成功關閉！", style="bold green")
            else:
                console.print(f"❌ {command_type} 執行失敗", style="bold red")

        # 如果有 raw_payload，嘗試解碼並查找位置信息
        if 'raw_payload' in ack_event:
            try:
                raw = ack_event['raw_payload']
                decoded = base64.b64decode(raw).decode(errors='ignore')

                # 查找位置相關信息
                if 'Latitude' in decoded and 'Longitude' in decoded:
                    console.print("📍 發現位置資訊！", style="bold green")
                    console.print(decoded)
                elif 'LostModeEnabled' in decoded:
                    console.print("🔍 發現遺失模式狀態資訊！", style="bold blue")
                    console.print(decoded)
                else:
                    console.print("[原始回應] 解碼的 raw_payload：", style="bold cyan")
                    console.print(decoded)

            except Exception as e:
                console.print(f"[解碼錯誤] {str(e)}", style="bold red")


def select_devices():
    # 先嘗試線上取得裝置
    status_code = get_device_from_net(MDM_URL, API_KEY, DEVICE_LIST_CSV)
    if status_code != 200:
        # 線上失敗則用本地方式
        console.print("⚠️ 線上取得裝置失敗，改用本地 mdmctl！", style="bold yellow")
        run_mdmctl_get_devices(DEVICE_LIST_CSV)

    devices = []
    with open(DEVICE_LIST_CSV, newline='') as csvfile:
        reader = csv.reader(csvfile)
        for row in reader:
            if len(row) >= 2:
                udid, serial = row[0].strip(), row[1].strip()
                if udid and serial:
                    devices.append((udid, serial))

    table = Table(title="📋 裝置清單：")
    table.add_column("序號", justify="right", style="cyan")
    table.add_column("SerialNumber", style="green")
    table.add_column("UDID", style="blue")
    for idx, (udid, serial) in enumerate(devices, 1):
        table.add_row(str(idx), serial, udid)
    console.print(table)

    return devices


def select_devices_with_filter(filter_option=None):
    devices = select_devices()

    if not filter_option:
        filter_option = Prompt.ask(
            "📦 請選擇操作方式 (1=全部, 2=選擇, 3=過濾)",
            choices=["1", "2", "3"],
            default="1"
        )
        console.print("1 = 所有裝置, 2 = 自選裝置（輸入序號）, 3 = 依序號過濾")

    if filter_option == "2":
        serial_input = Prompt.ask("請輸入要操作的序號（可用逗號分隔多筆）")
        selected = [int(s.strip()) for s in serial_input.split(',')]
        devices = [d for idx, d in enumerate(devices, 1) if idx in selected]
    elif filter_option == "3":
        filter_serial = Prompt.ask("請輸入要過濾的序號關鍵字")
        devices = [d for d in devices if filter_serial.lower() in d[1].lower()]

        # 顯示過濾後的結果
        table = Table(title=f"📋 過濾後的裝置清單 (關鍵字: {filter_serial})：")
        table.add_column("序號", justify="right", style="cyan")
        table.add_column("SerialNumber", style="green")
        table.add_column("UDID", style="blue")
        for idx, (udid, serial) in enumerate(devices, 1):
            table.add_row(str(idx), serial, udid)
        console.print(table)

        if not devices:
            console.print("⚠️ 沒有符合條件的裝置。", style="bold yellow")
            return []

        confirm = Confirm.ask("要繼續操作這些裝置嗎?", default=True)
        if not confirm:
            return []

    return devices

def show_menu():
    menu_table = Table(title="🎛️ MicroMDM 管理工具", show_header=False, box=None)
    menu_table.add_column("編號", style="cyan")
    menu_table.add_column("功能", style="green")

    menu_items = [
        ("1", "🚀 部署 VPP App"),
        ("2", "📱 部署企業內部 App"),
        ("3", "🔒 鎖定裝置"),
        ("4", "📩 傳送訊息 (透過鎖定顯示)"),
        ("5", "🔄 重開機"),
        ("6", "⚡ 關機"),
        ("7", "🔓 清除密碼"),
        ("8", "🧹 移除應用程式"),
        ("9", "💥 擦除裝置"),
        ("10", "📊 查詢裝置資訊"),
        ("11", "📋 查詢已安裝 App 清單"),
        ("12", "📋 查詢已安裝描述檔清單"),
        ("13", "📦 查詢可用系統更新"),
        ("14", "📲 排程系統更新"),
        ("15", "📝 安裝設定描述檔"),
        ("16", "🗑️ 移除設定描述檔"),
        ("17", "👤 設定裝置預設帳號"),
        ("18", "✅ 標記裝置已完成設定"),
        ("19", "🔑 獲取啟用鎖繞過碼"),
        ("20", "🔐 獲取安全資訊"),
        ("21", "🔐 獲取憑證清單"),
        ("22", "🧹 清除命令佇列"),
        ("23", "🔍 檢查命令佇列"),
        ("24", "🔔 發送 Push 通知"),
        ("25", "🔄 同步 DEP 裝置"),
        ("26", "🔍 啟用遺失模式"),
        ("27", "🔓 關閉遺失模式"),
        ("28", "📍 獲取設備位置（遺失模式）"),
        ("29", "🔊 播放遺失模式聲音"),
        ("30", "🔍 檢查遺失模式狀態"),
        ("0", "退出")
    ]

    for num, desc in menu_items:
        menu_table.add_row(num, desc)

    console.print(menu_table)

    choice = Prompt.ask("請選擇功能", default="0")
    return choice


def main():
    while True:
        socketio_thread = start_socketio_client()

        choice = show_menu()
        global response, devices
        if choice == "0":
            console.print("👋 程式結束", style="bold green")
            break

        # 大部分選項需要選擇裝置
        if choice in [
            "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
            "20", "21", "22", "23", "26", "27", "28", "29", "30"
        ]:
            devices = select_devices_with_filter()
            if not devices:
                console.print("⚠️ 沒有符合條件的裝置，返回主選單", style="bold yellow")
                continue

        # VPP App 安裝
        if choice == "1":
            app_input = Prompt.ask("📱 請輸入 App 的 URL 或 ID")
            app_id = parse_app_id(app_input)
            sToken = load_sToken(VPPTOKEN_PATH)
            for udid, serial in devices:
                assign_vpp_license(sToken, app_id, serial)
                response = install_app_to_device(MDM_URL, API_KEY, udid, app_id)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 企業內部 App 安裝
        elif choice == "2":
            identifier = Prompt.ask("請輸入要安裝的 App 識別碼（Bundle ID）")
            for udid, _ in devices:
                response = install_enterprise_app(MDM_URL, API_KEY, udid, identifier)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 鎖定裝置
        elif choice == "3":
            pin = Prompt.ask("🔐 請輸入鎖定 PIN（留空則不設定密碼）", default="")
            for udid, _ in devices:
                response = lock_device(MDM_URL, API_KEY, udid, pin if pin else None)
                if response == 201:
                    send_push_to_device(MDM_URL, API_KEY, udid)
                    info = wait_device_info(MDM_URL, API_KEY, udid, max_retry=20, sleep_time=10)
                    if info:
                        console.print(f"✅ 裝置資訊 ({udid}):", style="bold green")
                        console.print(json.dumps(info, ensure_ascii=False, indent=2))
                    else:
                        console.print(f"❌ 查詢裝置資訊失敗（裝置未即時回報，請稍後再試）", style="bold red")
                else:
                    console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                    console.print(response)

        # 傳送訊息（透過鎖定顯示）
        elif choice == "4":
            message = Prompt.ask("📩 請輸入要顯示的訊息內容")
            pin = Prompt.ask("🔐 請輸入鎖定 PIN（留空則不設定密碼）", default="")
            for udid, _ in devices:
                response = lock_device(MDM_URL, API_KEY, udid, pin if pin else None, message)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 重開機
        elif choice == "5":
            for udid, _ in devices:
                response = restart_device(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response.status_code == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response.status_code)
                console.print(response.text)

        # 關機
        elif choice == "6":
            for udid, _ in devices:
                response = shutdown_device(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response.status_code == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response.status_code)
                console.print(response.text)

        # 清除密碼
        elif choice == "7":
            for udid, _ in devices:
                response = clear_passcode(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 移除應用程式
        elif choice == "8":
            remove_all = Confirm.ask("是否移除所有應用程式？", default=False)
            if remove_all:
                identifier = "*"
            else:
                identifier = Prompt.ask("請輸入要移除的應用程式識別碼 (Bundle ID)")
            for udid, _ in devices:
                response = remove_application(MDM_URL, API_KEY, udid, identifier)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 擦除裝置
        elif choice == "9":
            confirm = Confirm.ask("⚠️ 警告：此操作將抹除所有裝置數據！確定要繼續嗎？", default=False)
            if not confirm:
                console.print("已取消操作", style="bold yellow")
                continue
            pin = Prompt.ask("🔐 請輸入解鎖 PIN（留空則不設定）", default="")
            for udid, _ in devices:
                response = erase_device(MDM_URL, API_KEY, udid, pin if pin else None)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 查詢裝置資訊
        elif choice == "10":
            for udid, _ in devices:
                response = get_device_info(MDM_URL, API_KEY, udid)
                if response == 201:
                    send_push_to_device(MDM_URL, API_KEY, udid)
                else:
                    console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                    console.print(response)

        # 查詢已安裝 App 清單
        elif choice == "11":
            for udid, _ in devices:
                response = get_installed_apps(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 查詢已安裝描述檔清單
        elif choice == "12":
            for udid, _ in devices:
                response = get_profiles(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 查詢可用系統更新
        elif choice == "13":
            for udid, _ in devices:
                response = get_os_updates(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 排程系統更新
        elif choice == "14":
            product_key = Prompt.ask("請輸入產品金鑰 (例如: 012-34567-A)")
            product_version = Prompt.ask("請輸入版本號 (例如: 17.5.1)")
            install_actions = {
                "1": "InstallASAP",
                "2": "DownloadOnly",
                "3": "NotifyOnly",
                "4": "InstallLater",
                "5": "InstallForceRestart"
            }
            console.print("安裝動作選項:")
            for key, val in install_actions.items():
                console.print(f"{key}. {val}")
            action_choice = Prompt.ask("請選擇安裝動作", choices=list(install_actions.keys()), default="1")
            install_action = install_actions[action_choice]
            for udid, _ in devices:
                response = schedule_os_update(MDM_URL, API_KEY, udid, product_key, product_version, install_action)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 安裝設定描述檔
        elif choice == "15":
            profiles = [f for f in os.listdir(PROFILES_DIR) if f.endswith('.mobileconfig')]
            if not profiles:
                console.print(f"⚠️ 在 {PROFILES_DIR} 目錄下沒有找到 .mobileconfig 檔案", style="bold yellow")
                profile_path = Prompt.ask("請輸入描述檔的完整路徑")
            else:
                table = Table(title="📋 可用描述檔列表：")
                table.add_column("序號", justify="right", style="cyan")
                table.add_column("檔案名稱", style="green")
                for idx, profile in enumerate(profiles, 1):
                    table.add_row(str(idx), profile)
                console.print(table)
                profile_idx = int(Prompt.ask("請選擇描述檔序號", default="1"))
                if 1 <= profile_idx <= len(profiles):
                    profile_path = os.path.join(PROFILES_DIR, profiles[profile_idx - 1])
                else:
                    console.print("無效選擇", style="bold red")
                    continue
            for udid, _ in devices:
                response = install_profile(MDM_URL, API_KEY, udid, profile_path)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 移除設定描述檔
        elif choice == "16":
            identifier = Prompt.ask("請輸入要移除的描述檔識別碼 (PayloadIdentifier)")
            for udid, _ in devices:
                response = remove_profile(MDM_URL, API_KEY, udid, identifier)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 設定裝置預設帳號
        elif choice == "17":
            fullname = Prompt.ask("請輸入顯示名稱 (例如: John Appleseed)")
            username = Prompt.ask("請輸入使用者名稱 (例如: john)")
            lock_info = Confirm.ask("是否鎖定帳號資訊防止變更?", default=True)
            for udid, _ in devices:
                response = setup_account(MDM_URL, API_KEY, udid, fullname, username, lock_info)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 標記裝置已完成設定
        elif choice == "18":
            for udid, _ in devices:
                response = device_configured(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 獲取啟用鎖繞過碼
        elif choice == "19":
            for udid, _ in devices:
                response = get_activation_lock_bypass(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 獲取安全資訊
        elif choice == "20":
            for udid, _ in devices:
                response = get_security_info(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 獲取憑證清單
        elif choice == "21":
            for udid, _ in devices:
                response = get_certificate_list(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 清除命令佇列
        elif choice == "22":
            confirm = Confirm.ask("⚠️ 確定要清除命令佇列嗎？這將移除所有待處理命令！", default=False)
            if not confirm:
                console.print("已取消操作", style="bold yellow")
                continue
            for udid, _ in devices:
                response = clear_command_queue(MDM_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 檢查命令佇列
        elif choice == "23":
            for udid, _ in devices:
                response = inspect_command_queue(MDM_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 發送 Push 通知
        elif choice == "24":
            for udid, _ in devices:
                response = send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 同步 DEP 裝置
        elif choice == "25":
            response = sync_dep_devices(MDM_URL, API_KEY)
            if response == 200:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 啟用遺失模式
        elif choice == "26":
            message = Prompt.ask("📩 請輸入遺失模式顯示訊息", default="此裝置已遺失，請聯絡管理員")
            phone_number = Prompt.ask("📞 請輸入聯絡電話（可選）", default="")
            footnote = Prompt.ask("📝 請輸入備註（可選）", default="")

            for udid, _ in devices:
                response = enable_lost_mode(
                    MDM_URL, API_KEY, udid,
                    message,
                    phone_number if phone_number else None,
                    footnote if footnote else None
                )
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 關閉遺失模式
        elif choice == "27":
            confirm = Confirm.ask("⚠️ 確定要關閉遺失模式嗎？", default=False)
            if not confirm:
                console.print("已取消操作", style="bold yellow")
                continue

            for udid, _ in devices:
                response = disable_lost_mode(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 獲取設備位置（遺失模式）
        elif choice == "28":
            console.print("⚠️ 注意：此功能僅在設備處於遺失模式時可用", style="bold yellow")
            console.print("💡 建議：先使用選項 30 檢查遺失模式狀態", style="bold cyan")
            confirm = Confirm.ask("確定要獲取設備位置嗎？", default=True)
            if not confirm:
                console.print("已取消操作", style="bold yellow")
                continue

            for udid, _ in devices:
                response = get_device_location_with_check(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)

            console.print("📡 命令已發送，請注意觀察 SocketIO 回應...", style="bold cyan")
            console.print("💡 位置資訊將通過 webhook 回應顯示", style="bold blue")

        # 播放遺失模式聲音
        elif choice == "29":
            console.print("⚠️ 注意：此功能僅在設備處於遺失模式時可用", style="bold yellow")
            confirm = Confirm.ask("確定要播放遺失模式聲音嗎？", default=True)
            if not confirm:
                console.print("已取消操作", style="bold yellow")
                continue

            for udid, _ in devices:
                response = play_lost_mode_sound(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ 作業完成！設備將播放遺失模式聲音", style="bold green")
            else:
                console.print("❌ 作業失敗，詳細內容如下：", style="bold red")
                console.print(response)

        # 檢查遺失模式狀態
        elif choice == "30":
            console.print("🔍 正在檢查設備遺失模式狀態...", style="bold blue")
            for udid, _ in devices:
                response = check_lost_mode_status(MDM_URL, API_KEY, udid)
                send_push_to_device(MDM_URL, API_KEY, udid)

            console.print("📡 狀態查詢命令已發送，請等待設備回應...", style="bold cyan")
            console.print("💡 遺失模式狀態將通過 SocketIO 回應顯示", style="bold blue")

        # 詢問是否繼續
        if not Confirm.ask("是否繼續執行其他操作?", default=True):
            console.print("👋 程式結束", style="bold green")
            break


if __name__ == "__main__":
    main()
