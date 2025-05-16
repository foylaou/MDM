import os
import csv
import json
import base64
import requests
import subprocess
from dotenv import load_dotenv
from rich import print as rprint
from rich.table import Table
from rich.console import Console
from rich.prompt import Prompt, Confirm
import xml.dom.minidom
import time
from datetime import datetime

# 載入 .env 檔案
load_dotenv()

# 設定常數
VPPTOKEN_PATH = './ISHA_APP_token.vpptoken'
SERVER_URL = os.getenv('SERVER_URL', 'https://mdm.isafe.org.tw')
API_KEY = os.getenv('API_KEY')
DEVICE_LIST_CSV = './devices.csv'
MDMCTL_BIN = 'mdmctl'
PROFILES_DIR = './profiles'

# 確保目錄存在
os.makedirs(PROFILES_DIR, exist_ok=True)

console = Console()



def run_mdmctl_get_devices(output_file):
    console.print("📥 Fetching all device data...", style="bold blue")
    command = f"{MDMCTL_BIN} get devices"
    awk_cmd = "awk 'NR>1 && $1 != \"\" {print $1 \",\" $2}'"
    full_cmd = f"{command} | {awk_cmd}"
    with open(output_file, "w") as f:
        subprocess.run(full_cmd, shell=True, stdout=f)

def get_device_from_net(server_url, api_key, output_file):
    console.print("📥 Fetching all device data...", style="bold blue")
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
        console.print("✅ Successfully retrieved device data", style="green")
    else:
        console.print(f"❌ Error: {resp.status_code}", style="bold red")
        console.print(resp.text)

    return resp.status_code

def load_sToken(vpptoken_path):
    with open(vpptoken_path, 'r') as f:
        encoded = f.read().strip()
        return encoded


def assign_vpp_license(sToken, adamId, serialNumber):
    console.print(f"🔑 Assigning VPP license to serial number {serialNumber}...", style="bold green")
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
    console.print(f"🚀 Installing App to UDID={udid}...", style="bold blue")
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
    console.print(f"✅ MicroMDM Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def install_enterprise_app(server_url, api_key, udid, manifest_url):
    console.print(f"🚀 Installing enterprise App to UDID={udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "InstallEnterpriseApplication",
        "manifest_url": manifest_url
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ MicroMDM Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def lock_device(server_url, api_key, udid, pin=None, message=None):
    console.print(f"🔒 Locking device {udid}...", style="bold blue")
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
    console.print(f"✅ Lock result ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def restart_device(server_url, api_key, udid):
    console.print(f"🔄 Restarting {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RestartDevice"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Restart response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp


def clear_passcode(server_url, api_key, udid):
    console.print(f"🔓 Clearing passcode for {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ClearPasscode"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Clear passcode response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def erase_device(server_url, api_key, udid, pin=None):
    console.print(f"💥 Erasing device {udid}...", style="bold red")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "EraseDevice"
    }
    if pin:
        payload["pin"] = pin

    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Erase response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def remove_application(server_url, api_key, udid, identifier="*"):
    console.print(f"🧹 Removing application {identifier} from {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RemoveApplication",
        "identifier": identifier
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_device_info(server_url, api_key, udid):
    console.print(f"📊 Retrieving device information {udid}...", style="bold blue")
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
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_installed_apps(server_url, api_key, udid):
    console.print(f"📋 Retrieving installed applications list {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "InstalledApplicationList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_profiles(server_url, api_key, udid):
    console.print(f"📋 Retrieving installed profile list {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ProfileList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_os_updates(server_url, api_key, udid):
    console.print(f"🔍 Querying available OS updates {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "AvailableOSUpdates"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def schedule_os_update(server_url, api_key, udid, product_key, product_version, install_action="InstallASAP"):
    console.print(f"📲 Scheduling OS update for {udid}...", style="bold blue")
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
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def install_profile(server_url, api_key, udid, profile_path):
    console.print(f"📝 Installing profile to {udid}...", style="bold blue")
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
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def remove_profile(server_url, api_key, udid, identifier):
    console.print(f"🗑️ Removing profile {identifier} from {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "RemoveProfile",
        "identifier": identifier
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def setup_account(server_url, api_key, udid, fullname, username, lock_info=True):
    console.print(f"👤 Setting device account {username} to {udid}...", style="bold blue")
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
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def device_configured(server_url, api_key, udid):
    console.print(f"✅ Marking device as configured {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "DeviceConfigured",
        "request_requires_network_tether": False
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_activation_lock_bypass(server_url, api_key, udid):
    console.print(f"🔑 Retrieving Activation Lock bypass code {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "ActivationLockBypassCode"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_security_info(server_url, api_key, udid):
    console.print(f"🔒 Retrieving security info {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "SecurityInfo"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def get_certificate_list(server_url, api_key, udid):
    console.print(f"🔐 Retrieving certificate list {udid}...", style="bold blue")
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    payload = {
        "udid": udid,
        "request_type": "CertificateList"
    }
    resp = requests.post(f"{server_url}/v1/commands", headers=headers, auth=auth, data=json.dumps(payload))
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def clear_command_queue(server_url, api_key, udid):
    console.print(f"🧹 Clearing command queue {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.delete(f"{server_url}/v1/commands/{udid}", auth=auth)
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def inspect_command_queue(server_url, api_key, udid):
    console.print(f"🔍 Inspecting command queue {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.get(f"{server_url}/v1/commands/{udid}", auth=auth)
    console.print(f"✅ Response ({udid}):", resp.status_code, style="green")
    console.print(resp.text)
    return resp.status_code


def push_device_with_mdmctl(udid):
    result = subprocess.run(["mdmctl", "push", udid], capture_output=True, text=True)
    if result.returncode == 0:
        console.print(f"✅ mdmctl push succeeded ({udid})", style="green")
    else:
        console.print(f"❌ mdmctl push failed ({udid}):", style="red")
        console.print(result.stderr)


def send_push_to_device(server_url, api_key, udid):
    console.print(f"🔔 Sending Push notification to device {udid}...", style="bold blue")
    auth = ('micromdm', api_key)
    try:
        resp = requests.get(f"{server_url}/push/{udid}", auth=auth)
        console.print(resp.text)
        if resp.status_code == 200:
            console.print(f"✅ Push notification response ({udid}): 200", style="green")
        else:
            console.print(f"❌ Push failed, trying mdmctl push", style="bold yellow")
            push_device_with_mdmctl(udid)
    except Exception as e:
        console.print(f"⚠️ Push error: {str(e)}. Fallback to mdmctl push", style="bold yellow")
        push_device_with_mdmctl(udid)


def sync_dep_devices(server_url, api_key):
    console.print(f"🔄 Syncing DEP devices...", style="bold blue")
    auth = ('micromdm', api_key)
    resp = requests.post(f"{server_url}/v1/dep/syncnow", auth=auth)
    console.print(f"✅ Response: {resp.status_code}", style="green")
    console.print(resp.text)
    return resp.status_code


def parse_app_id(input_str):
    if input_str.startswith("http"):
        return input_str.split("id")[-1].split("?")[0]
    return input_str.strip()


def wait_device_info(server_url, api_key, udid, max_retry=5, sleep_time=4):
    headers = {"Content-Type": "application/json"}
    auth = ('micromdm', api_key)
    data = json.dumps({})
    for i in range(max_retry):
        resp_info = requests.get(f"{server_url}/v1/devices/{udid}", headers=headers, auth=auth,data=data)
        if resp_info.status_code == 200:
            return resp_info.json()
        else:
            console.print(f"Attempt {i+1}: No data yet, status {resp_info.status_code}", style="yellow")
            time.sleep(sleep_time)
    return None

def select_devices():
    # 先嘗試線上取得裝置
    status_code = get_device_from_net(SERVER_URL, API_KEY, DEVICE_LIST_CSV)
    if status_code != 200:
        # 線上失敗則用本地方式
        console.print("⚠️ Failed to fetch devices online, switching to local mdmctl!", style="bold yellow")
        run_mdmctl_get_devices(DEVICE_LIST_CSV)

    devices = []
    with open(DEVICE_LIST_CSV, newline='') as csvfile:
        reader = csv.reader(csvfile)
        for row in reader:
            if len(row) >= 2:
                udid, serial = row[0].strip(), row[1].strip()
                if udid and serial:
                    devices.append((udid, serial))

    table = Table(title="📋 Device List:")
    table.add_column("No.", justify="right", style="cyan")
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
            "📦 Please select operation mode (1=All, 2=Select, 3=Filter)",
            choices=["1", "2", "3"],
            default="1"
        )
        console.print("1 = All devices, 2 = Select devices (enter index), 3 = Filter by serial number")

    if filter_option == "2":
        serial_input = Prompt.ask("Please enter the device index(es) to operate on (comma separated for multiple)")
        selected = [int(s.strip()) for s in serial_input.split(',')]
        devices = [d for idx, d in enumerate(devices, 1) if idx in selected]
    elif filter_option == "3":
        filter_serial = Prompt.ask("Please enter the serial number keyword to filter")
        devices = [d for d in devices if filter_serial.lower() in d[1].lower()]

        # Show filtered result
        table = Table(title=f"📋 Filtered device list (keyword: {filter_serial}):")
        table.add_column("No.", justify="right", style="cyan")
        table.add_column("SerialNumber", style="green")
        table.add_column("UDID", style="blue")
        for idx, (udid, serial) in enumerate(devices, 1):
            table.add_row(str(idx), serial, udid)
        console.print(table)

        if not devices:
            console.print("⚠️ No devices match your criteria.", style="bold yellow")
            return []

        confirm = Confirm.ask("Proceed to operate on these devices?", default=True)
        if not confirm:
            return []

    return devices


def show_menu():
    menu_table = Table(title="🎛️ MicroMDM Management Tool", show_header=False, box=None)
    menu_table.add_column("No.", style="cyan")
    menu_table.add_column("Function", style="green")

    menu_items = [
        ("1", "🚀 Deploy VPP App"),
        ("2", "📱 Deploy Enterprise App"),
        ("3", "🔒 Lock Device"),
        ("4", "📩 Send Message (displayed via lock)"),
        ("5", "🔄 Restart Device"),
        ("6", "🔓 Clear Passcode"),
        ("7", "🧹 Remove Application"),
        ("8", "💥 Erase Device"),
        ("9", "📊 Query Device Info"),
        ("10", "📋 Query Installed App List"),
        ("11", "📋 Query Installed Profile List"),
        ("12", "📦 Query Available OS Updates"),
        ("13", "📲 Schedule OS Update"),
        ("14", "📝 Install Configuration Profile"),
        ("15", "🗑️ Remove Configuration Profile"),
        ("16", "👤 Set Default Device Account"),
        ("17", "✅ Mark Device as Configured"),
        ("18", "🔑 Get Activation Lock Bypass Code"),
        ("19", "🔐 Get Security Info"),
        ("20", "🔐 Get Certificate List"),
        ("21", "🧹 Clear Command Queue"),
        ("22", "🔍 Inspect Command Queue"),
        ("23", "🔔 Send Push Notification"),
        ("24", "🔄 Sync DEP Devices"),
        ("0", "Exit")
    ]

    for num, desc in menu_items:
        menu_table.add_row(num, desc)

    console.print(menu_table)

    choice = Prompt.ask("Please select a function", default="0")
    return choice


def main():
    while True:
        choice = show_menu()
        global response
        if choice == "0":
            console.print("👋 Program exited", style="bold green")
            break

        # 大部分選項需要選擇裝置
        if choice in [
            "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
            "20", "21", "22", "23"
        ]:
            devices = select_devices_with_filter()
            if not devices:
                console.print("⚠️ No matching devices found, returning to main menu", style="bold yellow")
                continue

        # VPP App 安裝
        if choice == "1":
            app_input = Prompt.ask("📱 Please enter the App URL or ID")
            app_id = parse_app_id(app_input)
            sToken = load_sToken(VPPTOKEN_PATH)
            for udid, serial in devices:
                assign_vpp_license(sToken, app_id, serial)
                response = install_app_to_device(SERVER_URL, API_KEY, udid, app_id)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 企業內部 App 安裝
        elif choice == "2":
            for udid, _ in devices:
                identifier = Prompt.ask("Please enter the App identifier (Bundle ID) to install")
                response = install_enterprise_app(SERVER_URL, API_KEY, udid, identifier)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 鎖定裝置
        elif choice == "3":
            pin = Prompt.ask("🔐 Please enter lock PIN (leave blank for none)", default="")
            for udid, _ in devices:
                response = lock_device(SERVER_URL, API_KEY, udid, pin if pin else None)
                if response == 201:
                    send_push_to_device(SERVER_URL, API_KEY, udid)
                    info = wait_device_info(SERVER_URL, API_KEY, udid, max_retry=20, sleep_time=10)
                    if info:
                        console.print(f"✅ Device info ({udid}):", style="bold green")
                        console.print(json.dumps(info, ensure_ascii=False, indent=2))
                    else:
                        console.print(f"❌ Failed to retrieve device info (device did not report in time, please try again later)", style="bold red")
                else:
                    console.print("❌ Operation failed. Details below:", style="bold red")
                    console.print(response)


        # 傳送訊息（透過鎖定顯示）
        elif choice == "4":
            message = Prompt.ask("📩 Please enter the message to display")
            pin = Prompt.ask("🔐 Please enter lock PIN (leave blank for none)", default="")
            for udid, _ in devices:
                response = lock_device(SERVER_URL, API_KEY, udid, pin if pin else None, message)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 重開機
        elif choice == "5":
            for udid, _ in devices:
                response = restart_device(SERVER_URL, API_KEY, udid)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response.status_code == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response.status_code)
                console.print(response.text)

        # 清除密碼
        elif choice == "6":
            for udid, _ in devices:
                response = clear_passcode(SERVER_URL, API_KEY, udid)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 移除應用程式
        elif choice == "7":
            remove_all = Confirm.ask("Remove all applications?", default=False)
            if remove_all:
                identifier = "*"
            else:
                identifier = Prompt.ask("Please enter the application identifier (Bundle ID) to remove")
            for udid, _ in devices:
                response = remove_application(SERVER_URL, API_KEY, udid, identifier)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 擦除裝置
        elif choice == "8":
            confirm = Confirm.ask("⚠️ WARNING: This operation will erase all device data! Are you sure you want to continue?", default=False)
            if not confirm:
                console.print("Operation cancelled", style="bold yellow")
                continue
            pin = Prompt.ask("🔐 Please enter unlock PIN (leave blank for none)", default="")
            for udid, _ in devices:
                response = erase_device(SERVER_URL, API_KEY, udid, pin if pin else None)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)



        # 查詢裝置資訊
        elif choice == "9":
            for udid, _ in devices:
                response = get_device_info(SERVER_URL, API_KEY, udid)
                if response == 201:
                    send_push_to_device(SERVER_URL, API_KEY, udid)
                    info = wait_device_info(SERVER_URL, API_KEY, udid, max_retry=20, sleep_time=10)
                    if info:
                        console.print(f"✅ Device info ({udid}):", style="bold green")
                        console.print(json.dumps(info, ensure_ascii=False, indent=2))
                    else:
                        console.print(f"❌ Failed to retrieve device info (device did not report in time, please try again later)", style="bold red")
                else:
                    console.print("❌ Operation failed. Details below:", style="bold red")
                    console.print(response)

        # 查詢已安裝 App 清單
        elif choice == "10":
            for udid, _ in devices:
                response = get_installed_apps(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 查詢已安裝描述檔清單
        elif choice == "11":
            for udid, _ in devices:
                response = get_profiles(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 查詢可用系統更新
        elif choice == "12":
            for udid, _ in devices:
                response = get_os_updates(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 排程系統更新
        elif choice == "13":
            product_key = Prompt.ask("Please enter the product key (e.g. 012-34567-A)")
            product_version = Prompt.ask("Please enter the version number (e.g. 17.5.1)")
            install_actions = {
                "1": "InstallASAP",
                "2": "DownloadOnly",
                "3": "NotifyOnly",
                "4": "InstallLater",
                "5": "InstallForceRestart"
            }
            console.print("Install action options:")
            for key, val in install_actions.items():
                console.print(f"{key}. {val}")
            action_choice = Prompt.ask("Please select install action", choices=list(install_actions.keys()), default="1")
            install_action = install_actions[action_choice]
            for udid, _ in devices:
                response = schedule_os_update(SERVER_URL, API_KEY, udid, product_key, product_version, install_action)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 安裝設定描述檔
        elif choice == "14":
            profiles = [f for f in os.listdir(PROFILES_DIR) if f.endswith('.mobileconfig')]
            if not profiles:
                console.print(f"⚠️ No .mobileconfig files found in {PROFILES_DIR}", style="bold yellow")
                profile_path = Prompt.ask("Please enter the full path to the profile")
            else:
                table = Table(title="📋 Available Profiles:")
                table.add_column("No.", justify="right", style="cyan")
                table.add_column("Filename", style="green")
                for idx, profile in enumerate(profiles, 1):
                    table.add_row(str(idx), profile)
                console.print(table)
                profile_idx = int(Prompt.ask("Please select profile index", default="1"))
                if 1 <= profile_idx <= len(profiles):
                    profile_path = os.path.join(PROFILES_DIR, profiles[profile_idx - 1])
                else:
                    console.print("Invalid selection", style="bold red")
                    continue
            for udid, _ in devices:
                response = install_profile(SERVER_URL, API_KEY, udid, profile_path)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 移除設定描述檔
        elif choice == "15":
            identifier = Prompt.ask("Please enter the profile identifier (PayloadIdentifier) to remove")
            for udid, _ in devices:
                response = remove_profile(SERVER_URL, API_KEY, udid, identifier)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 設定裝置預設帳號
        elif choice == "16":
            fullname = Prompt.ask("Please enter display name (e.g. John Appleseed)")
            username = Prompt.ask("Please enter username (e.g. john)")
            lock_info = Confirm.ask("Lock account information to prevent changes?", default=True)
            for udid, _ in devices:
                response = setup_account(SERVER_URL, API_KEY, udid, fullname, username, lock_info)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 標記裝置已完成設定
        elif choice == "17":
            for udid, _ in devices:
                response = device_configured(SERVER_URL, API_KEY, udid)
                send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 獲取啟用鎖繞過碼
        elif choice == "18":
            for udid, _ in devices:
                response = get_activation_lock_bypass(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 獲取安全資訊
        elif choice == "19":
            for udid, _ in devices:
                response = get_security_info(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 獲取憑證清單
        elif choice == "20":
            for udid, _ in devices:
                response = get_certificate_list(SERVER_URL, API_KEY, udid)
            if response == 201:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 清除命令佇列
        elif choice == "21":
            confirm = Confirm.ask("⚠️ Are you sure you want to clear the command queue? This will remove all pending commands!", default=False)
            if not confirm:
                console.print("Operation cancelled", style="bold yellow")
                continue
            for udid, _ in devices:
                response = clear_command_queue(SERVER_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 檢查命令佇列
        elif choice == "22":
            for udid, _ in devices:
                response = inspect_command_queue(SERVER_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 發送 Push 通知
        elif choice == "23":
            for udid, _ in devices:
                response = send_push_to_device(SERVER_URL, API_KEY, udid)
            if response == 200:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        # 同步 DEP 裝置
        elif choice == "24":
            response = sync_dep_devices(SERVER_URL, API_KEY)
            if response == 200:
                console.print("✅ Operation completed!", style="bold green")
            else:
                console.print("❌ Operation failed. Details below:", style="bold red")
                console.print(response)

        if not Confirm.ask("Continue with other operations?", default=True):
            console.print("👋 Program exited", style="bold green")
            break


if __name__ == "__main__":
    main()
