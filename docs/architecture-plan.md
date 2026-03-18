# MDM 管理平台 — 架構規劃書

> 版本：v1.0 | 日期：2026-03-18

---

## 1. 專案概述

### 1.1 現狀

目前系統為一支 Python CLI 腳本 (`main.py`，約 1,330 行)，透過 HTTP 直接呼叫 MicroMDM API 來管理 iPad 裝置。功能完整但缺乏視覺化介面，不適合多人協作與權限控管。

### 1.2 目標

將現有 CLI 工具轉型為**前後端分離的 Web 管理平台**，提供：

- 視覺化裝置管理介面
- 多使用者登入與角色權限控管 (RBAC)
- 即時事件監控 (WebSocket / Server Streaming)
- 操作稽核紀錄
- 容器化部署

---

## 2. 系統架構總覽

```
┌─────────────────────────────────────────────────────────┐
│                      使用者瀏覽器                         │
│                   React + DaisyUI 前端                    │
│                  (ConnectRPC-Web 客戶端)                   │
└──────────────────────┬──────────────────────────────────┘
                       │ HTTPS (gRPC-Web / Connect Protocol)
                       ▼
┌─────────────────────────────────────────────────────────┐
│                  Go ConnectRPC 後端                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐ │
│  │AuthService│ │DeviceSvc │ │CommandSvc│ │ EventSvc   │ │
│  └──────────┘ └──────────┘ └──────────┘ └────────────┘ │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │ VPPSvc   │ │ UserSvc  │ │ AuditSvc │                │
│  └──────────┘ └──────────┘ └──────────┘                │
│               ┌──────────────────┐                      │
│               │  Webhook Receiver │ ← MicroMDM 推播      │
│               └──────────────────┘                      │
└──────┬──────────────┬──────────────┬────────────────────┘
       │              │              │
       ▼              ▼              ▼
┌────────────┐ ┌────────────┐ ┌──────────────────┐
│ PostgreSQL │ │  MicroMDM  │ │ Apple VPP / DEP  │
│  (持久層)   │ │  Server    │ │   Services       │
└────────────┘ └────────────┘ └──────────────────┘
```

---

## 3. 技術選型

### 3.1 後端 (src/mdm.api)

| 類別 | 技術 | 選型理由 |
|------|------|----------|
| 語言 | Go 1.22+ | 高效能、原生並發、單一二進位部署 |
| API 框架 | ConnectRPC | 相容 gRPC 生態系、原生支援瀏覽器 (不需 Envoy Proxy) |
| 資料庫 | PostgreSQL 16 | 成熟穩定、JSONB 支援裝置資訊彈性儲存 |
| DB Driver | pgx/v5 | Go 最高效能的 PostgreSQL 驅動 |
| 認證 | JWT (golang-jwt/v5) | 無狀態認證，適合前後端分離架構 |
| 密碼雜湊 | Argon2id | 目前推薦的密碼雜湊演算法 |
| API 定義 | Protocol Buffers (buf) | 強型別、自動產生客戶端程式碼 |
| 容器化 | Docker + Docker Compose | 統一開發與部署環境 |

### 3.2 前端 (src/mdm.web)

| 類別 | 技術 | 選型理由 |
|------|------|----------|
| 框架 | React 19 | 生態系成熟、元件豐富 |
| 語言 | TypeScript 5.9 | 型別安全、提升開發體驗 |
| 建置工具 | Vite 7 | 極速 HMR、原生 ESM |
| UI 套件 | DaisyUI 5 + Tailwind CSS 4 | 基於 Nexus React 3.2.0 範本，元件豐富美觀 |
| API 客戶端 | @connectrpc/connect-web | 與後端 ConnectRPC 無縫整合 |
| 路由 | React Router 7 | 成熟穩定的 SPA 路由方案 |
| 表格 | @tanstack/react-table | 高效能虛擬化表格 |
| 圖表 | ApexCharts | 豐富的圖表類型 |
| 表單驗證 | Zod | TypeScript 優先的 Schema 驗證 |
| 狀態管理 | React Context API | 輕量簡潔，符合專案規模 |

### 3.3 共用工具

| 類別 | 技術 |
|------|------|
| Protobuf 工具鏈 | buf CLI |
| 版本控制 | Git |
| CI/CD | GitHub Actions (建議) |
| 反向代理 | Nginx / Caddy (生產環境) |

---

## 4. 後端架構設計

### 4.1 Clean Architecture 分層

```
cmd/server/main.go          ← 應用程式入口、依賴注入
│
├── proto/mdm/v1/            ← Protobuf 定義 (API 合約)
├── gen/mdm/v1/              ← buf 自動產生的程式碼
│
└── internal/
    ├── domain/              ← 領域實體 (純 Go 結構體，無外部依賴)
    │   ├── user.go
    │   ├── device.go
    │   └── audit.go
    │
    ├── port/                ← 介面定義 (依賴反轉)
    │   ├── user_repo.go
    │   ├── device_repo.go
    │   ├── audit_repo.go
    │   └── mdm_client.go
    │
    ├── config/              ← 環境變數載入
    │
    ├── middleware/           ← 橫切關注點
    │   ├── auth.go          ← JWT 驗證攔截器
    │   └── rbac.go          ← 角色權限檢查
    │
    ├── adapter/             ← 外部系統實作
    │   ├── postgres/        ← 資料庫存取層
    │   │   ├── user_repo.go
    │   │   ├── device_repo.go
    │   │   └── audit_repo.go
    │   ├── micromdm/        ← MicroMDM HTTP 客戶端
    │   │   └── client.go
    │   └── vpp/             ← Apple VPP 客戶端
    │       └── client.go
    │
    └── service/             ← ConnectRPC 服務實作
        ├── auth_service.go
        ├── device_service.go
        ├── command_service.go
        ├── event_service.go
        ├── event_broker.go
        ├── webhook.go
        ├── vpp_service.go
        ├── user_service.go
        └── audit_service.go
```

### 4.2 七大 gRPC 服務

#### AuthService — 認證服務
| RPC | 說明 | 權限 |
|-----|------|------|
| Login | 帳密登入，回傳 JWT | 公開 |
| RefreshToken | 更新 Access Token | 已認證 |
| ChangePassword | 修改密碼 | 已認證 |

#### DeviceService — 裝置管理
| RPC | 說明 | 權限 |
|-----|------|------|
| ListDevices | 列出裝置（支援篩選、分頁） | operator+ |
| GetDevice | 取得單一裝置詳情 | operator+ |
| SyncDevices | 從 MicroMDM 同步裝置清單 | admin |
| SyncDEPDevices | 同步 DEP 裝置 | admin |

#### CommandService — MDM 指令（22+ RPC）
| 類別 | RPC | 權限 |
|------|-----|------|
| 裝置控制 | LockDevice, RestartDevice, ShutdownDevice, ClearPasscode | operator+ |
| 高風險 | EraseDevice | admin |
| 應用管理 | InstallApp, InstallEnterpriseApp, RemoveApp | operator+ |
| 描述檔 | InstallProfile, RemoveProfile | operator+ |
| 資訊查詢 | GetDeviceInfo, GetInstalledApps, GetProfileList, GetSecurityInfo, GetCertificateList | operator+ |
| 系統更新 | GetAvailableOSUpdates, ScheduleOSUpdate | operator+ |
| 裝置設定 | SetupAccount, DeviceConfigured, GetActivationLockBypass | admin |
| 遺失模式 | EnableLostMode, DisableLostMode, GetDeviceLocation, PlayLostModeSound | operator+ |
| 佇列管理 | SendPush, ClearCommandQueue, InspectCommandQueue | operator+ |

#### EventService — 即時事件
| RPC | 說明 | 權限 |
|-----|------|------|
| StreamEvents | Server Streaming，即時推送 MicroMDM 事件 | 已認證 |

#### VPPService — Apple VPP 授權
| RPC | 說明 | 權限 |
|-----|------|------|
| AssignLicense | 批次指派 VPP App 授權 | operator+ |
| RevokeLicense | 撤銷授權 | operator+ |

#### UserService — 使用者管理
| RPC | 說明 | 權限 |
|-----|------|------|
| CreateUser | 建立使用者 | admin |
| ListUsers | 列出使用者 | admin |
| UpdateUser | 更新使用者 | admin |
| DeleteUser | 刪除使用者 | admin |

#### AuditService — 稽核日誌
| RPC | 說明 | 權限 |
|-----|------|------|
| ListAuditLogs | 查詢操作紀錄（支援篩選、分頁） | admin |

### 4.3 Webhook 即時事件流

```
MicroMDM Server
    │
    │ POST /webhook (Acknowledge/CheckOut 事件)
    ▼
Go Server (webhook.go)
    │
    │ 解析事件 → 更新 DB 裝置狀態
    │
    ▼
EventBroker (fan-out pub/sub)
    │
    │ 廣播至所有已訂閱的 StreamEvents 連線
    ▼
前端 (EventService.StreamEvents)
    │
    └─→ 即時更新 UI
```

---

## 5. 前端架構設計

### 5.1 基於 Nexus React 3.2.0 範本

採用 Nexus React 3.2.0 作為 UI 基礎範本，保留其：
- Admin Layout（側邊欄 + 頂部導覽 + 內容區）
- DaisyUI 主題系統（亮色/暗色/多種主題切換）
- 響應式設計
- 表格、表單、圖表等基礎元件

裁減不需要的範本頁面（電商、聊天、Landing Page 等），聚焦 MDM 功能。

### 5.2 目錄結構

```
src/mdm.web/
├── public/                          # 靜態資源
├── src/
│   ├── main.tsx                     # 應用入口
│   ├── router/
│   │   ├── index.tsx                # 路由設定
│   │   └── guard.tsx                # 路由守衛 (認證檢查)
│   │
│   ├── contexts/
│   │   ├── auth.tsx                 # 認證狀態 Context
│   │   └── config.tsx               # 主題/設定 Context (來自 Nexus)
│   │
│   ├── hooks/
│   │   ├── use-auth.ts              # 認證相關 Hook
│   │   ├── use-devices.ts           # 裝置查詢 Hook
│   │   └── use-local-storage.ts     # LocalStorage Hook
│   │
│   ├── lib/
│   │   └── client.ts               # ConnectRPC 客戶端設定
│   │
│   ├── components/
│   │   ├── layout/                  # 版面元件 (基於 Nexus admin-layout)
│   │   │   ├── AdminLayout.tsx
│   │   │   ├── Sidebar.tsx
│   │   │   ├── Topbar.tsx
│   │   │   └── Footer.tsx
│   │   ├── device/                  # 裝置相關元件
│   │   │   ├── DeviceCard.tsx
│   │   │   ├── DeviceTable.tsx
│   │   │   └── DeviceFilter.tsx
│   │   ├── command/                 # 指令相關元件
│   │   │   ├── CommandPanel.tsx
│   │   │   └── CommandConfirm.tsx
│   │   └── common/                  # 共用元件
│   │       ├── StatusBadge.tsx
│   │       └── ConfirmDialog.tsx
│   │
│   ├── pages/
│   │   ├── auth/
│   │   │   └── Login.tsx            # 登入頁
│   │   ├── dashboard/
│   │   │   └── Dashboard.tsx        # 儀表板總覽
│   │   ├── devices/
│   │   │   ├── DeviceList.tsx       # 裝置列表
│   │   │   └── DeviceDetail.tsx     # 裝置詳情
│   │   ├── commands/
│   │   │   └── Commands.tsx         # 指令操作中心
│   │   ├── profiles/
│   │   │   └── Profiles.tsx         # 描述檔管理
│   │   ├── apps/
│   │   │   └── AppManagement.tsx    # 應用程式管理
│   │   ├── events/
│   │   │   └── Events.tsx           # 即時事件監控
│   │   ├── audit/
│   │   │   └── AuditLogs.tsx        # 稽核日誌
│   │   └── settings/
│   │       └── Users.tsx            # 使用者管理
│   │
│   ├── gen/mdm/v1/                  # buf 產生的 TS 客戶端程式碼
│   │
│   └── styles/                      # 樣式 (基於 Nexus 樣式系統)
│       ├── app.css
│       └── tailwind.css
│
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
└── buf.gen.yaml                     # 前端 Protobuf 程式碼產生設定
```

### 5.3 頁面規劃

#### Dashboard — 儀表板
- 裝置狀態總覽（線上/離線/未註冊數量統計卡片）
- 裝置 OS 版本分佈圓餅圖
- 近期事件時間軸
- 快速操作入口

#### Devices — 裝置管理
- 裝置列表表格（支援搜尋、篩選、分頁）
  - 欄位：名稱、序號、型號、OS 版本、上次上線、狀態
- 裝置詳情頁
  - 基本資訊、安裝的 App、描述檔、憑證
  - 快速指令按鈕（鎖定、重啟、推播等）
- 批次選取 → 批次執行指令

#### Commands — 指令中心
- 指令分類選單（裝置控制 / App 管理 / 描述檔 / 資訊查詢 / 遺失模式）
- 目標裝置選擇（全部 / 多選 / 依序號篩選）
- 指令參數表單（依指令類型動態產生）
- 高風險指令二次確認對話框（EraseDevice 等）
- 指令佇列檢視

#### Events — 即時事件
- Server Streaming 即時事件串流
- 事件列表（含時間戳、裝置、事件類型、狀態）
- 事件篩選器
- 自動滾動 + 暫停

#### Apps — 應用程式管理
- VPP App 授權指派
- 企業 App (Enterprise) 安裝
- App 移除

#### Profiles — 描述檔管理
- 安裝描述檔（上傳 .mobileconfig）
- 移除描述檔
- 查看已安裝描述檔

#### Audit — 稽核日誌
- 操作紀錄表格（使用者、動作、目標、時間）
- 篩選條件（日期範圍、使用者、動作類型）
- 匯出功能

#### Settings — 系統設定
- 使用者管理（CRUD，僅 admin）
- 個人密碼修改

---

## 6. 資料庫設計

### 6.1 ER 圖

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────┐
│    users     │     │    audit_logs    │     │    devices     │
├──────────────┤     ├──────────────────┤     ├────────────────┤
│ id (PK)      │◄────│ user_id (FK)     │     │ udid (PK)      │
│ username     │     │ id (PK)          │     │ serial_number  │
│ password_hash│     │ username         │     │ device_name    │
│ role         │     │ action           │     │ model          │
│ display_name │     │ target           │     │ os_version     │
│ created_at   │     │ detail (JSONB)   │     │ last_seen      │
│ updated_at   │     │ timestamp        │     │ enrollment_status│
└──────────────┘     └──────────────────┘     │ dep_profile_status│
                                               │ details (JSONB) │
                                               │ created_at     │
                                               │ updated_at     │
                                               └────────────────┘
```

### 6.2 SQL Schema

```sql
-- 001_init.up.sql
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(64) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          VARCHAR(16) NOT NULL DEFAULT 'viewer',  -- admin | operator | viewer
    display_name  VARCHAR(128) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
    udid              VARCHAR(64) PRIMARY KEY,
    serial_number     VARCHAR(32) UNIQUE NOT NULL,
    device_name       VARCHAR(256) NOT NULL DEFAULT '',
    model             VARCHAR(128) NOT NULL DEFAULT '',
    os_version        VARCHAR(32) NOT NULL DEFAULT '',
    last_seen         TIMESTAMPTZ,
    enrollment_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    dep_profile_status VARCHAR(32) NOT NULL DEFAULT '',
    details           JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id        BIGSERIAL PRIMARY KEY,
    user_id   UUID REFERENCES users(id),
    username  VARCHAR(64) NOT NULL,
    action    VARCHAR(64) NOT NULL,
    target    VARCHAR(256) NOT NULL DEFAULT '',
    detail    JSONB NOT NULL DEFAULT '{}',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_timestamp ON audit_logs (timestamp DESC);
CREATE INDEX idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX idx_devices_serial ON devices (serial_number);

-- 初始管理員帳號 (密碼: admin，上線後須立即修改)
INSERT INTO users (username, password_hash, role, display_name)
VALUES ('admin', '$argon2id$...', 'admin', '系統管理員')
ON CONFLICT DO NOTHING;
```

---

## 7. 認證與授權

### 7.1 JWT 流程

```
1. POST Login(username, password)
   → 驗證 Argon2id 雜湊
   → 簽發 Access Token (15min) + Refresh Token (7d)

2. 每次 API 請求
   → Authorization: Bearer <access_token>
   → JWT 攔截器驗證簽名與過期時間
   → 注入 user_id, role 至 Context

3. Token 過期
   → 前端攔截 401
   → 自動呼叫 RefreshToken
   → 取得新 Access Token
```

### 7.2 RBAC 角色權限矩陣

| 功能 | admin | operator | viewer |
|------|:-----:|:--------:|:------:|
| 檢視裝置列表 | v | v | v |
| 檢視裝置詳情 | v | v | v |
| 檢視即時事件 | v | v | v |
| 執行一般指令 | v | v | x |
| 安裝/移除 App | v | v | x |
| 安裝/移除描述檔 | v | v | x |
| 清除裝置 (Erase) | v | x | x |
| 同步 DEP 裝置 | v | x | x |
| 使用者管理 | v | x | x |
| 檢視稽核日誌 | v | x | x |

---

## 8. 部署架構

### 8.1 Docker Compose (開發/測試)

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: mdm
      POSTGRES_USER: mdm
      POSTGRES_PASSWORD: mdm
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  api:
    build: ./src/mdm.api
    depends_on:
      - postgres
    environment:
      DATABASE_URL: postgres://mdm:mdm@postgres:5432/mdm?sslmode=disable
      JWT_SECRET: ${JWT_SECRET}
      MICROMDM_URL: ${MICROMDM_URL}
      MICROMDM_API_KEY: ${MICROMDM_API_KEY}
    ports:
      - "8080:8080"

  web:
    build: ./src/mdm.web
    depends_on:
      - api
    ports:
      - "3000:80"

volumes:
  pgdata:
```

### 8.2 生產環境架構

```
                    Internet
                       │
                       ▼
              ┌──────────────┐
              │  Nginx/Caddy │  ← TLS 終止、靜態檔案、反向代理
              │  (Port 443)  │
              └──────┬───────┘
                     │
          ┌──────────┴──────────┐
          │                     │
          ▼                     ▼
   /api/* → Go Server     /* → React SPA
   (Port 8080)             (靜態檔案)
          │
          ▼
   PostgreSQL (Port 5432)
```

**Nginx 關鍵設定：**
```nginx
location /api/ {
    proxy_pass http://api:8080/;
    # ConnectRPC 需要 HTTP/2 或 gRPC-Web headers
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}

location / {
    root /usr/share/nginx/html;
    try_files $uri $uri/ /index.html;  # SPA fallback
}
```

---

## 9. 開發工作流程

### 9.1 Protobuf 程式碼產生

```bash
# 安裝 buf CLI
# https://buf.build/docs/installation

# 產生 Go 伺服器端程式碼
cd src/mdm.api && buf generate

# 產生 TypeScript 客戶端程式碼
cd src/mdm.web && buf generate
```

### 9.2 本地開發

```bash
# 1. 啟動 PostgreSQL
cd src/mdm.api && docker-compose up -d postgres

# 2. 啟動後端 (含 DB Migration)
cd src/mdm.api && go run ./cmd/server

# 3. 啟動前端 (Vite Dev Server)
cd src/mdm.web && npm install && npm run dev
```

### 9.3 開發原則

- **API 優先**：先定義 .proto，再寫實作
- **型別安全**：前後端共用 Protobuf 產生的型別定義
- **增量遷移**：逐步將 main.py 的功能搬移至 Web 介面，兩者可並存

---

## 10. 實作里程碑

### Phase 1：基礎建設 (核心骨架)
- [x] 定義 Protobuf API 合約（7 個 Service）
- [x] 建立 Go 後端專案結構 (Clean Architecture)
- [x] 建立 React 前端專案結構
- [ ] 完成 DB Migration 與種子資料
- [ ] 完成 JWT 認證流程 (Login / Refresh / Middleware)
- [ ] 完成前端 Login 頁面 + AuthContext
- [ ] 完成 Admin Layout（側邊欄 + 路由）

### Phase 2：裝置管理 (核心功能)
- [ ] DeviceService — 裝置列表 / 詳情 / 同步
- [ ] 前端 — 裝置列表頁（表格 + 搜尋 + 分頁）
- [ ] 前端 — 裝置詳情頁
- [ ] MicroMDM Adapter — HTTP 客戶端實作

### Phase 3：指令系統
- [ ] CommandService — 22+ MDM 指令實作
- [ ] 前端 — 指令操作中心
- [ ] 前端 — 批次指令 + 確認對話框
- [ ] 稽核日誌寫入

### Phase 4：即時事件 + 進階功能
- [ ] Webhook 接收 → EventBroker → StreamEvents
- [ ] 前端 — 即時事件監控頁
- [ ] VPP App 授權管理
- [ ] 描述檔上傳與管理

### Phase 5：管理與運維
- [ ] 使用者管理 (CRUD)
- [ ] 稽核日誌查詢頁
- [ ] Dashboard 儀表板（統計圖表）
- [ ] 生產環境 Docker 建置 + Nginx 設定
- [ ] CI/CD Pipeline

---

## 11. CLI → Web 功能對照表

| # | main.py 功能 | 對應頁面 | 後端 Service | 狀態 |
|---|-------------|---------|-------------|------|
| 1 | 部署 VPP App | Apps | CommandService.InstallApp | 待開發 |
| 2 | 部署企業 App | Apps | CommandService.InstallEnterpriseApp | 待開發 |
| 3 | 鎖定裝置 | Devices / Commands | CommandService.LockDevice | 待開發 |
| 4 | 發送訊息 | Commands | CommandService.LockDevice (with msg) | 待開發 |
| 5 | 重啟裝置 | Devices / Commands | CommandService.RestartDevice | 待開發 |
| 6 | 關機 | Commands | CommandService.ShutdownDevice | 待開發 |
| 7 | 清除密碼 | Commands | CommandService.ClearPasscode | 待開發 |
| 8 | 清除裝置 | Commands | CommandService.EraseDevice | 待開發 |
| 9 | 安裝描述檔 | Profiles | CommandService.InstallProfile | 待開發 |
| 10 | 移除描述檔 | Profiles | CommandService.RemoveProfile | 待開發 |
| 11 | 取得裝置資訊 | Devices | CommandService.GetDeviceInfo | 待開發 |
| 12 | 取得已安裝 App | Devices | CommandService.GetInstalledApps | 待開發 |
| 13 | 取得描述檔列表 | Devices | CommandService.GetProfileList | 待開發 |
| 14 | 取得可用更新 | Devices / Commands | CommandService.GetAvailableOSUpdates | 待開發 |
| 15 | 排程 OS 更新 | Commands | CommandService.ScheduleOSUpdate | 待開發 |
| 16 | 取得安全資訊 | Devices | CommandService.GetSecurityInfo | 待開發 |
| 17 | 取得憑證列表 | Devices | CommandService.GetCertificateList | 待開發 |
| 18 | 設定預設帳號 | Commands | CommandService.SetupAccount | 待開發 |
| 19 | 標記已設定 | Commands | CommandService.DeviceConfigured | 待開發 |
| 20 | 啟動鎖定繞過碼 | Commands | CommandService.GetActivationLockBypass | 待開發 |
| 21 | 發送 Push | Commands | CommandService.SendPush | 待開發 |
| 22 | 清除指令佇列 | Commands | CommandService.ClearCommandQueue | 待開發 |
| 23 | 檢查指令佇列 | Commands | CommandService.InspectCommandQueue | 待開發 |
| 24 | 同步 DEP 裝置 | Devices | DeviceService.SyncDEPDevices | 待開發 |
| 25 | 啟用遺失模式 | Devices / Commands | CommandService.EnableLostMode | 待開發 |
| 26 | 關閉遺失模式 | Devices / Commands | CommandService.DisableLostMode | 待開發 |
| 27 | 取得裝置位置 | Devices | CommandService.GetDeviceLocation | 待開發 |
| 28 | 播放遺失模式聲音 | Devices / Commands | CommandService.PlayLostModeSound | 待開發 |
| 29 | 移除 App | Apps | CommandService.RemoveApp | 待開發 |
| 30 | VPP 授權指派 | Apps | VPPService.AssignLicense | 待開發 |

---

## 12. 安全考量

| 項目 | 措施 |
|------|------|
| 傳輸安全 | 生產環境強制 HTTPS (TLS 1.3) |
| 密碼儲存 | Argon2id 雜湊，不可逆 |
| 認證 | JWT，Access Token 短期效期 (15min) |
| 授權 | 服務端 RBAC，前端路由守衛為輔助 |
| 高風險操作 | EraseDevice 限 admin + 前端二次確認 |
| 稽核 | 所有寫入操作記錄至 audit_logs |
| CORS | 僅允許指定 Origin |
| API Key 保護 | MicroMDM API Key 僅存於後端環境變數，不暴露給前端 |
| 輸入驗證 | Protobuf 型別 + Zod 前端驗證 |
| 依賴安全 | 定期更新依賴、使用 `govulncheck` / `npm audit` |

---

## 附錄 A：環境變數清單

| 變數 | 必填 | 預設值 | 說明 |
|------|:----:|--------|------|
| `LISTEN_ADDR` | | `:8080` | 後端監聽位址 |
| `DATABASE_URL` | v | | PostgreSQL 連線字串 |
| `JWT_SECRET` | v | | JWT 簽名密鑰 (至少 32 字元) |
| `MICROMDM_URL` | v | | MicroMDM 伺服器網址 |
| `MICROMDM_API_KEY` | v | | MicroMDM API 金鑰 |
| `VPP_TOKEN_PATH` | | | Apple VPP sToken 檔案路徑 |
| `WEBHOOK_PATH` | | `/webhook` | Webhook 端點路徑 |
| `VITE_API_BASE_URL` | | `http://localhost:8080` | 前端 API 基礎網址 |
