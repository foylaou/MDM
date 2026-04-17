# MDM 管理平台 v2 — 模組化改版設計書

> 版本：v2.0-draft | 日期：2026-04-16
> 狀態：**Phase 0–3 完成（Phase 4、5 待做）**

---

## 1. 改版目標

### 1.1 核心需求

| # | 需求 | 說明 |
|---|------|------|
| 1 | 模組拆分 | 將系統拆為「財產管理」「MDM 裝置管理」「租借系統」三大模組 |
| 2 | 模組化權限 | 每個模組獨立的角色權限控管，符合職責分離原則 |
| 3 | Email 通知 | 租借申請、核准/拒絕、逾期催還等自動寄信通知 |
| 4 | ISO 27001 對齊 | 以 ISO/IEC 27001:2022 Annex A 控制項為稽核標準 |
| 5 | UI 模組區分 | 前端依模組分區，清晰的導覽結構 |

### 1.2 設計原則

- **漸進式重構**：不砍掉重練，在現有 codebase 上演進
- **單體內模組化**：邏輯拆分為模組，但仍是同一個部署單元（不拆微服務）
- **最小權限原則**：使用者只能存取被授權的模組與操作
- **稽核可追溯**：所有關鍵操作皆記錄，可追溯到人、時間、操作內容

---

## 2. 模組拆分定義

### 2.1 三大模組總覽

```
┌─────────────────────────────────────────────────────────────────┐
│                        MDM 管理平台 v2                           │
│                                                                 │
│  ┌─────────────────┐ ┌─────────────────┐ ┌──────────────────┐  │
│  │   財產管理模組    │ │  MDM 裝置管理模組 │ │   租借系統模組    │  │
│  │   (Asset)       │ │  (MDM)          │ │   (Rental)       │  │
│  ├─────────────────┤ ├─────────────────┤ ├──────────────────┤  │
│  │ • 財產清冊      │ │ • 裝置列表/詳情  │ │ • 租借申請       │  │
│  │ • 資產分類      │ │ • MDM 指令       │ │ • 審批流程       │  │
│  │ • 保管人管理    │ │ • App 管理       │ │ • 借出/歸還      │  │
│  │ • 盤點作業      │ │ • 描述檔管理     │ │ • 逾期追蹤       │  │
│  │ • 資產生命週期  │ │ • VPP 授權       │ │ • Email 通知     │  │
│  │ • 報廢/移撥     │ │ • 即時事件       │ │ • 匯出/存查      │  │
│  └────────┬────────┘ └────────┬────────┘ └────────┬─────────┘  │
│           │                   │                    │            │
│  ┌────────┴───────────────────┴────────────────────┴─────────┐  │
│  │                   共用基礎設施層                             │  │
│  │  認證(Auth) │ 權限(RBAC) │ 稽核(Audit) │ 通知(Notify)      │  │
│  │  使用者管理  │ 系統設定    │ Email 服務  │ Dashboard        │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 模組邊界與職責

#### 模組 A：財產管理 (Asset Management)

| 項目 | 說明 |
|------|------|
| **目的** | 管理組織的 IT 資產全生命週期，滿足 ISO 27001 A.5.9 資產清冊要求 |
| **範圍** | 財產清冊 CRUD、分類管理、保管人指派、盤點、報廢/移撥 |
| **資料擁有** | `assets`, `categories`, 資產相關欄位 |
| **對外依賴** | 讀取 `devices` 資訊以建立資產與裝置的關聯 |

**核心功能：**
- 財產清冊（資產編號、名稱、規格、單價、取得日期、保管人、存放地點）
- 資產分類（樹狀分類：品牌 > 產品線 > 型號）
- 保管人管理（指派/變更保管人，記錄異動歷史）
- 盤點作業（產生盤點清單、記錄盤點結果、差異報告）
- 資產生命週期（採購 → 啟用 → 使用中 → 報廢/移撥/遺失）
- 報表匯出（Excel 財產清冊、盤點報告）

#### 模組 B：MDM 裝置管理 (MDM & Device Management)

| 項目 | 說明 |
|------|------|
| **目的** | 透過 Apple MDM 協定遠端管理裝置，執行配置與監控 |
| **範圍** | 裝置同步/查看、MDM 指令執行、App/描述檔管理、VPP、即時事件 |
| **資料擁有** | `devices`, MDM 相關指令與事件 |
| **對外依賴** | MicroMDM Server、Apple VPP/DEP 服務 |

**核心功能：** （與現有 v1 相同，不變動）
- 裝置列表/詳情/同步
- 22+ MDM 指令
- App 安裝/移除 + VPP 授權
- 描述檔安裝/移除
- 即時事件串流
- DEP 裝置同步

#### 模組 C：租借系統 (Rental/Lending System)

| 項目 | 說明 |
|------|------|
| **目的** | 管理裝置租借全流程，含申請、審批、通知，滿足 ISO 27001 A.5.10 可接受使用 |
| **範圍** | 租借申請、審批流程、借出/歸還、Email 通知、逾期追蹤 |
| **資料擁有** | `rentals`, `notifications` |
| **對外依賴** | 讀取 `devices` + `assets`（檢查保管人）、Email 服務 |

**核心功能：**
- 租借申請（選擇裝置、填寫用途、預計歸還日）
- 審批流程（保管人/主管核准 → admin 確認借出）
- 歸還流程（清點檢查表 → 確認歸還）
- **Email 通知**（新功能）：
  - 新申請通知 → 保管人/核准人
  - 核准/拒絕通知 → 借用人
  - 借出確認通知 → 借用人
  - 逾期催還通知 → 借用人 + 保管人
  - 歸還確認通知 → 保管人
- 逾期追蹤（自動排程檢查）
- 匯出 Excel + 存查

---

## 3. 權限模型改版

### 3.1 從全域角色到模組權限

**v1（現行）：** 全域三角色

```
admin → 所有權限
operator → 操作權限（不含使用者管理、稽核）
viewer → 只讀
```

**v2（改版）：** 全域角色 + 模組級權限

```
使用者 = 全域角色 (system_role) + 每模組權限 (module_permissions)
```

### 3.2 全域角色（系統層級）

| 角色 | 說明 | 權限 |
|------|------|------|
| `sys_admin` | 系統管理員 | 使用者管理、系統設定、所有模組的管理者權限 |
| `user` | 一般使用者 | 依模組權限決定可存取範圍 |

### 3.3 模組權限（Module Permissions）

每個使用者可對每個模組擁有不同的存取層級：

| 模組 | 權限層級 | 說明 |
|------|----------|------|
| **asset** (財產管理) | `none` | 無法存取此模組 |
| | `viewer` | 查看財產清冊 |
| | `operator` | 新增/編輯資產、變更保管人 |
| | `manager` | 報廢/移撥審核、盤點管理 |
| **mdm** (裝置管理) | `none` | 無法存取此模組 |
| | `viewer` | 查看裝置列表、詳情 |
| | `operator` | 執行 MDM 指令（不含 Erase） |
| | `manager` | 高風險指令（Erase）、DEP 同步 |
| **rental** (租借系統) | `none` | 無法存取此模組 |
| | `requester` | 僅能申請租借（自己的） |
| | `approver` | 核准/拒絕租借申請（保管人角色） |
| | `manager` | 完整管理權限（借出、匯出、存查） |

### 3.4 權限矩陣範例

| 功能 | sys_admin | asset:manager | asset:operator | asset:viewer | mdm:manager | mdm:operator | mdm:viewer | rental:manager | rental:approver | rental:requester |
|------|:---------:|:-------------:|:--------------:|:------------:|:-----------:|:------------:|:----------:|:--------------:|:---------------:|:----------------:|
| 查看財產清冊 | v | v | v | v | - | - | - | - | - | - |
| 編輯資產 | v | v | v | - | - | - | - | - | - | - |
| 報廢/移撥 | v | v | - | - | - | - | - | - | - | - |
| 查看裝置列表 | v | - | - | - | v | v | v | - | - | - |
| 執行 MDM 指令 | v | - | - | - | v | v | - | - | - | - |
| 清除裝置 | v | - | - | - | v | - | - | - | - | - |
| 申請租借 | v | - | - | - | - | - | - | v | v | v |
| 核准/拒絕 | v | - | - | - | - | - | - | v | v | - |
| 借出/歸還/匯出 | v | - | - | - | - | - | - | v | - | - |

### 3.5 職責分離（Segregation of Duties）— ISO 27001 A.5.3

| 原則 | 實作方式 |
|------|----------|
| 財產管理者不一定有 MDM 權限 | 模組權限獨立，互不影響 |
| 租借申請人不能自己核准 | 系統強制：`requester` 不含 `approver` 權限 |
| 保管人核准不等同管理員借出 | 審批為兩階段：保管人核准 → manager 確認借出 |
| 稽核日誌不可被操作人員修改 | `audit_logs` 只能 INSERT，不可 UPDATE/DELETE |

---

## 4. Email 通知服務設計

### 4.1 架構

```
┌──────────────────┐
│  租借系統 / 其他  │
│  業務模組         │
└────────┬─────────┘
         │ 呼叫 NotifyService
         ▼
┌──────────────────┐
│  NotifyService   │
│  (通知服務層)     │
│  • 決定收件人    │
│  • 套用模板      │
│  • 寫入通知記錄  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  EmailAdapter    │
│  (SMTP 寄信)     │
│  • Go net/smtp   │
│  • 支援 TLS      │
│  • 支援 HTML     │
└──────────────────┘
```

### 4.2 通知觸發情境

| 事件 | 收件人 | 信件主旨 | 內容重點 |
|------|--------|----------|----------|
| 新租借申請 | 保管人 / 核准人 | [MDM] 新租借申請待核准 — #{單號} | 借用人、裝置清單、用途、申請時間 |
| 申請核准 | 借用人 | [MDM] 您的租借申請已核准 — #{單號} | 核准人、預計借出日 |
| 申請拒絕 | 借用人 | [MDM] 您的租借申請已拒絕 — #{單號} | 拒絕原因 |
| 裝置借出 | 借用人 | [MDM] 裝置已借出 — #{單號} | 裝置清單、預計歸還日、使用注意事項 |
| 逾期催還 | 借用人 + 保管人 | [MDM] 租借逾期提醒 — #{單號} | 逾期天數、裝置清單 |
| 裝置歸還 | 保管人 | [MDM] 裝置已歸還 — #{單號} | 歸還清點結果、備註 |

### 4.3 Email 模板

使用 Go `html/template`，模板存放於 `internal/notify/templates/`：

```
internal/notify/
├── notify.go          # NotifyService 介面 + 實作
├── email_adapter.go   # SMTP 寄信實作
└── templates/
    ├── rental_request.html
    ├── rental_approved.html
    ├── rental_rejected.html
    ├── rental_activated.html
    ├── rental_overdue.html
    └── rental_returned.html
```

### 4.4 通知記錄表

所有發出的通知皆記錄於 DB（供稽核）：

```sql
CREATE TABLE IF NOT EXISTS notifications (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type          TEXT NOT NULL,          -- 'email'
    event         TEXT NOT NULL,          -- 'rental_request', 'rental_approved', ...
    recipient     TEXT NOT NULL,          -- email 地址
    subject       TEXT NOT NULL,
    body          TEXT NOT NULL,          -- rendered HTML
    status        TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'sent', 'failed'
    error_message TEXT,
    reference_id  TEXT,                   -- 關聯的租借 ID 等
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ
);

CREATE INDEX idx_notifications_event ON notifications(event);
CREATE INDEX idx_notifications_status ON notifications(status);
CREATE INDEX idx_notifications_reference ON notifications(reference_id);
```

### 4.5 環境變數

| 變數 | 必填 | 預設值 | 說明 |
|------|:----:|--------|------|
| `SMTP_HOST` | v | | SMTP 伺服器位址 |
| `SMTP_PORT` | | `587` | SMTP 連線埠 |
| `SMTP_USERNAME` | v | | SMTP 帳號 |
| `SMTP_PASSWORD` | v | | SMTP 密碼 |
| `SMTP_FROM` | v | | 寄件人地址（如 `mdm@company.com`） |
| `SMTP_FROM_NAME` | | `MDM 管理平台` | 寄件人顯示名稱 |
| `SMTP_TLS` | | `true` | 是否使用 TLS |

---

## 5. 資料庫 Schema 變更

### 5.1 新增：使用者模組權限表

```sql
-- 007_module_permissions.up.sql

-- 使用者模組權限
CREATE TABLE IF NOT EXISTS user_module_permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module      TEXT NOT NULL,  -- 'asset', 'mdm', 'rental'
    permission  TEXT NOT NULL,  -- 'viewer', 'operator', 'manager', 'requester', 'approver'
    granted_by  UUID REFERENCES users(id),
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, module)
);

CREATE INDEX idx_ump_user ON user_module_permissions(user_id);
CREATE INDEX idx_ump_module ON user_module_permissions(module);

-- users 表新增 system_role 欄位（取代原 role）
ALTER TABLE users ADD COLUMN IF NOT EXISTS system_role TEXT NOT NULL DEFAULT 'user';
-- 遷移：將現有 admin 設為 sys_admin，其他設為 user
UPDATE users SET system_role = 'sys_admin' WHERE role = 'admin';
UPDATE users SET system_role = 'user' WHERE role != 'admin';

-- 為現有使用者建立預設模組權限（根據舊 role 遷移）
-- admin → 所有模組 manager
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, m.module, 'manager'
FROM users, (VALUES ('asset'), ('mdm'), ('rental')) AS m(module)
WHERE role = 'admin'
ON CONFLICT DO NOTHING;

-- operator → mdm:operator, rental:manager, asset:operator
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'mdm', 'operator' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'rental', 'manager' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'asset', 'operator' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;

-- viewer → mdm:viewer, rental:requester, asset:viewer
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'mdm', 'viewer' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'rental', 'requester' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'asset', 'viewer' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;
```

### 5.2 新增：使用者 email 欄位

```sql
-- 007_module_permissions.up.sql (續)

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';
```

### 5.3 新增：通知記錄表

```sql
-- 007_module_permissions.up.sql (續)

CREATE TABLE IF NOT EXISTS notifications (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type          TEXT NOT NULL DEFAULT 'email',
    event         TEXT NOT NULL,
    recipient     TEXT NOT NULL,
    subject       TEXT NOT NULL,
    body          TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    reference_id  TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_event ON notifications(event);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);
CREATE INDEX IF NOT EXISTS idx_notifications_reference ON notifications(reference_id);
```

### 5.4 資產生命週期欄位擴充

```sql
-- 007_module_permissions.up.sql (續)

-- 資產狀態與生命週期
ALTER TABLE assets ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
-- status: 'procurement' | 'active' | 'in_repair' | 'disposed' | 'transferred' | 'lost'

ALTER TABLE assets ADD COLUMN IF NOT EXISTS disposed_at TIMESTAMPTZ;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS disposed_by UUID REFERENCES users(id);
ALTER TABLE assets ADD COLUMN IF NOT EXISTS dispose_reason TEXT NOT NULL DEFAULT '';
```

### 5.5 完整 ER 圖（v2）

```
┌──────────────┐     ┌──────────────────────┐     ┌────────────────┐
│    users     │     │ user_module_perms     │     │    devices     │
├──────────────┤     ├──────────────────────┤     ├────────────────┤
│ id (PK)      │◄────│ user_id (FK)          │     │ udid (PK)      │
│ username     │     │ id (PK)               │     │ serial_number  │
│ email (new)  │     │ module                │     │ device_name    │
│ password_hash│     │ permission            │     │ ...            │
│ system_role  │     │ granted_by (FK)       │     └───────┬────────┘
│ role (legacy)│     │ granted_at            │             │
│ ...          │     └──────────────────────┘             │
└──────┬───────┘                                          │
       │                                                  │
       │     ┌──────────────┐     ┌──────────────┐       │
       │     │   assets     │────▶│  categories  │       │
       │     ├──────────────┤     └──────────────┘       │
       │     │ id (PK)      │                             │
       ├────▶│ custodian_id │                             │
       │     │ device_udid  │─────────────────────────────┘
       │     │ status (new) │
       │     │ ...          │
       │     └──────────────┘
       │
       │     ┌──────────────┐     ┌───────────────┐
       │     │   rentals    │     │ notifications │
       │     ├──────────────┤     ├───────────────┤
       ├────▶│ borrower_id  │     │ id (PK)       │
       ├────▶│ approver_id  │     │ type          │
       │     │ device_udid  │─────│ event         │
       │     │ ...          │     │ recipient     │
       │     └──────────────┘     │ status        │
       │                          │ reference_id  │
       │     ┌──────────────┐     │ ...           │
       └────▶│  audit_logs  │     └───────────────┘
             └──────────────┘
```

---

## 6. 後端 Clean Architecture 重構

### 6.1 現狀問題

目前 `cmd/server/main.go` 有 **2318 行**，包含：
- 40 個 REST endpoint 全部以 inline anonymous function 寫在 `main()`
- 業務邏輯（SQL query、資料轉換）直接寫在 handler 裡
- `*pgxpool.Pool` 直接在 handler 中使用，繞過 repository 層
- 沒有 controller / handler 層的抽象
- 認證檢查重複寫在每個 handler（`middleware.ExtractTokenFromRequest`）

**這違反了 Clean Architecture 的核心原則：**
- 框架細節（HTTP handler）直接耦合業務邏輯
- 資料存取沒有經過 repository 介面
- 無法單元測試（handler 直接存取 DB pool）

### 6.2 Clean Architecture 分層

```
┌────────────────────────────────────────────────────────────────────┐
│                         Presentation 層                            │
│                   (Controller / HTTP Handler)                      │
│                                                                    │
│  接收 HTTP 請求 → 解析參數 → 呼叫 Service → 回傳 JSON              │
│  不包含業務邏輯，不直接存取 DB                                       │
└──────────────────────────────┬─────────────────────────────────────┘
                               │ 呼叫
                               ▼
┌────────────────────────────────────────────────────────────────────┐
│                         Application 層                             │
│                      (Service / Use Case)                          │
│                                                                    │
│  業務邏輯：審批流程、權限檢查、通知觸發、資料組合                     │
│  透過 Port 介面存取外部資源（不直接依賴實作）                         │
└──────────────────────────────┬─────────────────────────────────────┘
                               │ 透過介面
                               ▼
┌────────────────────────────────────────────────────────────────────┐
│                          Domain 層                                 │
│                    (Entity / Value Object)                          │
│                                                                    │
│  純 Go struct，無外部依賴                                           │
│  領域實體：User, Device, Asset, Rental, Notification                │
└────────────────────────────────────────────────────────────────────┘
                               ▲
                               │ 實作介面
┌────────────────────────────────────────────────────────────────────┐
│                        Infrastructure 層                           │
│                      (Adapter / Repository)                        │
│                                                                    │
│  postgres/ — DB 存取實作                                           │
│  micromdm/ — MicroMDM HTTP 客戶端                                  │
│  vpp/ — Apple VPP 客戶端                                           │
│  smtp/ — Email 寄信實作                                            │
└────────────────────────────────────────────────────────────────────┘
```

**依賴方向**：Controller → Service → Port(介面) ← Adapter(實作)

### 6.3 目錄結構（重構後）

```
src/mdm.api/
├── cmd/server/
│   └── main.go                    # 瘦身至 ~100 行：只做依賴注入 + 路由掛載
│
├── internal/
│   ├── domain/                    # Domain 層 — 純實體
│   │   ├── user.go
│   │   ├── device.go
│   │   ├── asset.go               # 擴充 status 等
│   │   ├── rental.go              # 新增
│   │   ├── notification.go        # 新增
│   │   └── permission.go          # 新增（模組權限常數與型別）
│   │
│   ├── port/                      # Port 層 — 介面定義
│   │   ├── repository.go          # 所有 Repository 介面
│   │   ├── mdm_client.go          # MicroMDM 客戶端介面
│   │   ├── vpp_client.go          # VPP 客戶端介面
│   │   └── email_sender.go        # Email 寄信介面
│   │
│   ├── service/                   # Application 層 — 業務邏輯
│   │   ├── auth_service.go        # 登入、Token、密碼
│   │   ├── user_service.go        # 使用者 CRUD + 模組權限管理
│   │   ├── device_service.go      # 裝置查詢、同步
│   │   ├── asset_service.go       # 新增：財產 CRUD、保管人變更、生命週期
│   │   ├── rental_service.go      # 新增：租借全流程 + 通知觸發
│   │   ├── app_service.go         # 新增：managed apps CRUD、安裝/移除
│   │   ├── category_service.go    # 新增：分類 CRUD
│   │   ├── notify_service.go      # 新增：通知寄信 + 記錄
│   │   ├── command_service.go     # ConnectRPC 指令（保留）
│   │   ├── event_service.go       # ConnectRPC 事件（保留）
│   │   ├── event_broker.go        # 保留
│   │   ├── vpp_service.go         # ConnectRPC VPP（保留）
│   │   ├── audit_service.go       # ConnectRPC 稽核（保留）
│   │   └── webhook.go             # 保留
│   │
│   ├── controller/                # Presentation 層 — HTTP Handler（新增）
│   │   ├── auth_controller.go     # /api/login, /api/logout, /api/me, /api/register, /api/setup
│   │   ├── device_controller.go   # /api/devices/*, /api/devices-list, /api/devices-available
│   │   ├── asset_controller.go    # /api/assets, /api/assets/:id, /api/device-status
│   │   ├── rental_controller.go   # /api/rentals, /api/rentals/:id/*, /api/rentals-archive
│   │   ├── app_controller.go      # /api/managed-apps, /api/device-apps/*, /api/itunes-*
│   │   ├── category_controller.go # /api/categories, /api/categories/:id
│   │   ├── user_controller.go     # /api/users/*, /api/users-list
│   │   ├── profile_controller.go  # /api/profiles, /api/profiles/:id
│   │   ├── notification_controller.go # /api/notifications
│   │   ├── system_controller.go   # /api/system-status, /api/ws-config, /health
│   │   └── router.go              # 路由註冊：組裝所有 controller 到 mux
│   │
│   ├── middleware/                 # 橫切關注點
│   │   ├── auth.go                # JWT 驗證（現有，修改支援 system_role）
│   │   ├── module_auth.go         # 新增：模組權限中間件
│   │   └── audit_logger.go        # 新增：自動記錄稽核日誌（IP、User-Agent）
│   │
│   ├── adapter/                   # Infrastructure 層 — 外部實作
│   │   ├── postgres/
│   │   │   ├── user_repo.go       # 現有
│   │   │   ├── device_repo.go     # 現有
│   │   │   ├── audit_repo.go      # 現有
│   │   │   ├── asset_repo.go      # 擴充
│   │   │   ├── rental_repo.go     # 新增（從 main.go 提取）
│   │   │   ├── app_repo.go        # 新增（從 main.go 提取）
│   │   │   ├── category_repo.go   # 新增（從 main.go 提取）
│   │   │   ├── permission_repo.go # 新增
│   │   │   └── notification_repo.go # 新增
│   │   ├── micromdm/              # 不變
│   │   ├── vpp/                   # 不變
│   │   └── smtp/                  # 新增
│   │       └── sender.go
│   │
│   ├── notify/                    # Email 模板
│   │   └── templates/
│   │       ├── rental_request.html
│   │       ├── rental_approved.html
│   │       ├── rental_rejected.html
│   │       ├── rental_activated.html
│   │       ├── rental_overdue.html
│   │       └── rental_returned.html
│   │
│   ├── config/                    # 不變
│   └── db/                        # 不變
│
├── proto/                         # 不變
└── gen/                           # 不變
```

### 6.4 各層職責與範例

#### Controller 層 — 只做「拆包 → 呼叫 → 打包」

```go
// internal/controller/rental_controller.go

type RentalController struct {
    rentalSvc *service.RentalService
    auth      *middleware.AuthHelper  // 抽取 token、檢查權限
}

func NewRentalController(rentalSvc *service.RentalService, auth *middleware.AuthHelper) *RentalController {
    return &RentalController{rentalSvc: rentalSvc, auth: auth}
}

// RegisterRoutes 將路由掛載到 mux
func (c *RentalController) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("/api/rentals", c.handleRentals)
    mux.HandleFunc("/api/rentals/", c.handleRentalByID)
    mux.HandleFunc("/api/rentals-archive", c.handleArchive)
}

// handleRentals — GET: 列表, POST: 新增
func (c *RentalController) handleRentals(w http.ResponseWriter, r *http.Request) {
    claims, err := c.auth.RequireModule(r, "rental", "requester")
    if err != nil {
        writeError(w, http.StatusForbidden, "insufficient permissions")
        return
    }

    switch r.Method {
    case http.MethodGet:
        filter := service.RentalFilter{
            Status:      r.URL.Query().Get("status"),
            ShowArchived: r.URL.Query().Get("show_archived") == "true",
        }
        rentals, err := c.rentalSvc.List(r.Context(), claims, filter)
        if err != nil {
            writeError(w, http.StatusInternalServerError, err.Error())
            return
        }
        writeJSON(w, map[string]interface{}{"rentals": rentals})

    case http.MethodPost:
        var req service.CreateRentalRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, http.StatusBadRequest, "invalid request body")
            return
        }
        result, err := c.rentalSvc.Create(r.Context(), claims, req)
        if err != nil {
            writeError(w, http.StatusBadRequest, err.Error())
            return
        }
        writeJSON(w, result)
    }
}
```

#### Service 層 — 業務邏輯集中處

```go
// internal/service/rental_service.go

type RentalService struct {
    rentalRepo  port.RentalRepository
    deviceRepo  port.DeviceRepository
    assetRepo   port.AssetRepository
    userRepo    port.UserRepository
    auditRepo   port.AuditRepository
    notifySvc   *NotifyService
}

type CreateRentalRequest struct {
    DeviceUDIDs    []string `json:"device_udids"`
    BorrowerID     string   `json:"borrower_id"`
    Purpose        string   `json:"purpose"`
    ExpectedReturn *string  `json:"expected_return"`
    Notes          string   `json:"notes"`
}

func (s *RentalService) Create(ctx context.Context, claims *middleware.Claims, req CreateRentalRequest) (*CreateRentalResult, error) {
    // 1. 驗證裝置是否可借（檢查 asset status）
    // 2. 取得保管人資訊
    // 3. 建立 rental 記錄
    // 4. 寫入稽核日誌
    // 5. 觸發通知 → 保管人/核准人
    s.notifySvc.SendRentalRequest(ctx, rental, custodian)
    return result, nil
}

func (s *RentalService) Approve(ctx context.Context, claims *middleware.Claims, rentalID string) error {
    // 1. 檢查權限（必須是保管人或 manager）
    // 2. 更新狀態 pending → approved
    // 3. 寫入稽核日誌
    // 4. 觸發通知 → 借用人
    s.notifySvc.SendRentalApproved(ctx, rental, borrower)
    return nil
}
```

#### Port 層 — 新增 Repository 介面

```go
// internal/port/repository.go

// RentalRepository — 從 main.go 提取的租借資料存取
type RentalRepository interface {
    Create(ctx context.Context, rental *domain.Rental) error
    GetByID(ctx context.Context, id string) (*domain.Rental, error)
    List(ctx context.Context, filter RentalFilter) ([]*domain.Rental, error)
    UpdateStatus(ctx context.Context, id string, status string, approverID *string) error
    Archive(ctx context.Context, ids []string) error
    ListOverdue(ctx context.Context) ([]*domain.Rental, error)
}

// AppRepository — managed apps + device_apps
type AppRepository interface {
    ListManagedApps(ctx context.Context) ([]*domain.ManagedApp, error)
    CreateManagedApp(ctx context.Context, app *domain.ManagedApp) (string, error)
    UpdateManagedApp(ctx context.Context, id string, app *domain.ManagedApp) error
    DeleteManagedApp(ctx context.Context, id string) error
    ListDeviceApps(ctx context.Context, udid string) ([]*domain.DeviceApp, error)
    AddDeviceApp(ctx context.Context, udid string, appID string) error
    RemoveDeviceApp(ctx context.Context, udid string, appID string) error
}

// CategoryRepository
type CategoryRepository interface {
    List(ctx context.Context) ([]*domain.Category, error)
    Create(ctx context.Context, cat *domain.Category) (string, error)
    Update(ctx context.Context, id string, cat *domain.Category) error
    Delete(ctx context.Context, id string) error
}

// PermissionRepository — 模組權限
type PermissionRepository interface {
    GetByUserID(ctx context.Context, userID string) ([]*domain.ModulePermission, error)
    Set(ctx context.Context, userID string, module string, permission string, grantedBy string) error
    Delete(ctx context.Context, userID string, module string) error
}

// NotificationRepository
type NotificationRepository interface {
    Create(ctx context.Context, notif *domain.Notification) error
    UpdateStatus(ctx context.Context, id string, status string, errMsg string) error
    List(ctx context.Context, filter NotificationFilter) ([]*domain.Notification, error)
}

// EmailSender — SMTP 寄信介面
type EmailSender interface {
    Send(ctx context.Context, to string, subject string, htmlBody string) error
}
```

### 6.5 main.go 重構後

```go
// cmd/server/main.go — 目標 ~120 行

func main() {
    cfg := config.Load()

    // --- Infrastructure ---
    pool := initDB(cfg.DatabaseURL)
    defer pool.Close()
    runMigrations(pool)

    mdmClient := micromdm.NewClient(cfg.MicroMDMURL, cfg.MicroMDMKey)
    vppClient, _ := vpp.NewClient(cfg.VPPTokenPath)
    emailSender := smtp.NewSender(cfg.SMTP)  // 可為 nil（未配置時不寄信）

    // --- Repositories ---
    userRepo := postgres.NewUserRepo(pool)
    deviceRepo := postgres.NewDeviceRepo(pool)
    auditRepo := postgres.NewAuditRepo(pool)
    assetRepo := postgres.NewAssetRepo(pool)
    rentalRepo := postgres.NewRentalRepo(pool)
    appRepo := postgres.NewAppRepo(pool)
    categoryRepo := postgres.NewCategoryRepo(pool)
    permissionRepo := postgres.NewPermissionRepo(pool)
    notificationRepo := postgres.NewNotificationRepo(pool)

    // --- Services ---
    broker := service.NewEventBroker()
    notifySvc := service.NewNotifyService(emailSender, notificationRepo, userRepo)
    authSvc := service.NewAuthService(userRepo, permissionRepo, cfg.JWTSecret)
    deviceSvc := service.NewDeviceService(mdmClient, deviceRepo, auditRepo)
    assetSvc := service.NewAssetService(assetRepo, deviceRepo, auditRepo)
    rentalSvc := service.NewRentalService(rentalRepo, deviceRepo, assetRepo, userRepo, auditRepo, notifySvc)
    appSvc := service.NewAppService(appRepo, mdmClient, vppClient, auditRepo)
    categorySvc := service.NewCategoryService(categoryRepo)
    userMgmtSvc := service.NewUserManagementService(userRepo, permissionRepo, auditRepo)
    commandSvc := service.NewCommandService(mdmClient, vppClient, auditRepo, broker, assetRepo, deviceRepo)

    // --- Auth Helper ---
    authHelper := middleware.NewAuthHelper(cfg.JWTSecret, permissionRepo)

    // --- Controllers ---
    authCtrl := controller.NewAuthController(authSvc, authHelper, cfg)
    deviceCtrl := controller.NewDeviceController(deviceSvc, authHelper)
    assetCtrl := controller.NewAssetController(assetSvc, authHelper)
    rentalCtrl := controller.NewRentalController(rentalSvc, authHelper)
    appCtrl := controller.NewAppController(appSvc, authHelper)
    categoryCtrl := controller.NewCategoryController(categorySvc, authHelper)
    userCtrl := controller.NewUserController(userMgmtSvc, authHelper)
    notifCtrl := controller.NewNotificationController(notifySvc, authHelper)
    systemCtrl := controller.NewSystemController(cfg)

    // --- Router ---
    mux := http.NewServeMux()
    controller.RegisterAll(mux,
        authCtrl, deviceCtrl, assetCtrl, rentalCtrl,
        appCtrl, categoryCtrl, userCtrl, notifCtrl, systemCtrl,
    )

    // ConnectRPC services（保留）
    registerConnectRPC(mux, commandSvc, deviceSvc, /* ... */)

    // Webhook + SocketIO relay
    registerWebhook(mux, cfg, broker, deviceRepo)

    // Start server
    startServer(cfg.ListenAddr, mux)
}
```

### 6.6 Router 統一註冊

```go
// internal/controller/router.go

// Registrable 所有 controller 實作此介面
type Registrable interface {
    RegisterRoutes(mux *http.ServeMux)
}

// RegisterAll 一次掛載所有 controller 的路由
func RegisterAll(mux *http.ServeMux, controllers ...Registrable) {
    for _, c := range controllers {
        c.RegisterRoutes(mux)
    }
}
```

### 6.7 模組權限中間件

```go
// internal/middleware/module_auth.go

type AuthHelper struct {
    jwtSecret      string
    permissionRepo port.PermissionRepository
}

// RequireAuth — 僅驗證 JWT，回傳 claims
func (h *AuthHelper) RequireAuth(r *http.Request) (*Claims, error) { ... }

// RequireModule — 驗證 JWT + 檢查模組權限
// minLevel: "viewer" | "requester" | "operator" | "approver" | "manager"
func (h *AuthHelper) RequireModule(r *http.Request, module string, minLevel string) (*Claims, error) {
    claims, err := h.RequireAuth(r)
    if err != nil {
        return nil, err
    }
    // sys_admin 自動通過
    if claims.SystemRole == "sys_admin" {
        return claims, nil
    }
    // 查詢使用者在此模組的權限層級
    perm, err := h.permissionRepo.GetByUserAndModule(r.Context(), claims.UserID, module)
    if err != nil || !isLevelSufficient(perm.Level, minLevel) {
        return nil, ErrInsufficientPermission
    }
    return claims, nil
}

// RequireSysAdmin — 限 sys_admin
func (h *AuthHelper) RequireSysAdmin(r *http.Request) (*Claims, error) { ... }

// 權限層級比較
var levelOrder = map[string]int{
    "none": 0, "viewer": 1, "requester": 2,
    "operator": 3, "approver": 4, "manager": 5,
}

func isLevelSufficient(actual, required string) bool {
    return levelOrder[actual] >= levelOrder[required]
}
```

### 6.8 Controller 共用工具

```go
// internal/controller/helpers.go

func writeJSON(w http.ResponseWriter, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseID(r *http.Request, prefix string) string {
    return strings.TrimPrefix(r.URL.Path, prefix)
}
```

### 6.9 重構前後對比

| 面向 | 重構前 (v1) | 重構後 (v2) |
|------|------------|------------|
| main.go | **2318 行**，40 個 inline handler | **~120 行**，只做依賴注入 + 掛載 |
| 業務邏輯 | 散落在 handler 匿名函數中 | 集中在 `service/` 層 |
| DB 存取 | handler 直接 `pool.Query()` | 經過 `Repository` 介面 |
| 認證檢查 | 每個 handler 重複寫 `ExtractTokenFromRequest` | `AuthHelper` 統一處理 |
| 模組權限 | 無（只有全域 role） | `RequireModule()` 中間件 |
| 可測試性 | 無法 mock DB | 可 mock `port.Repository` 介面 |
| 路由管理 | 全部塞在 main() | 每個 controller 自行 `RegisterRoutes` |

### 6.10 REST 端點完整對照

```
Controller              端點                            權限要求                    來源
──────────────────────────────────────────────────────────────────────────────────────────
auth_controller
                        POST   /api/login              公開                        main.go:171
                        POST   /api/logout             公開                        main.go:226
                        GET    /api/me                 認證                        main.go:236
                        POST   /api/register           公開                        main.go:250
                        POST   /api/setup              公開(僅首次)                main.go:310

system_controller
                        GET    /health                 公開                        main.go:164
                        GET    /api/system-status      公開                        main.go:293
                        GET    /api/ws-config          認證                        main.go:361

device_controller
                        GET    /api/devices/:udid      mdm:viewer                  main.go:373
                        GET    /api/devices-list       mdm:viewer                  main.go:411
                        GET    /api/devices-available  rental:requester            main.go:520
                        PUT    /api/device-status      asset:operator              main.go:578
                        POST   /api/sync-device-info   mdm:manager                 main.go:614

asset_controller
                        GET    /api/assets             asset:viewer                main.go:652
                        POST   /api/assets             asset:operator              main.go:652
                        PUT    /api/assets/:id         asset:operator              main.go:793
                        DELETE /api/assets/:id         asset:manager               main.go:793

app_controller
                        GET    /api/managed-apps       mdm:viewer                  main.go:909
                        POST   /api/managed-apps       mdm:operator                main.go:909
                        PUT    /api/managed-apps/:id   mdm:operator                main.go:1005
                        DELETE /api/managed-apps/:id   mdm:operator                main.go:1005
                        GET    /api/device-apps        mdm:viewer                  main.go:1066
                        POST   /api/device-apps/install   mdm:operator             main.go:1121
                        POST   /api/device-apps/update    mdm:operator             main.go:1258
                        POST   /api/device-apps/uninstall mdm:operator             main.go:1363
                        POST   /api/sync-device-apps      mdm:operator             main.go:1432
                        GET    /api/itunes-lookup      mdm:viewer                  main.go:851
                        GET    /api/itunes-search      mdm:viewer                  main.go:881

rental_controller
                        GET    /api/rentals            rental:requester            main.go:1620
                        POST   /api/rentals            rental:requester            main.go:1620
                        POST   /api/rentals/:id/approve   rental:approver          main.go:1802
                        POST   /api/rentals/:id/reject    rental:approver          main.go:1802
                        POST   /api/rentals/:id/activate  rental:manager           main.go:1802
                        POST   /api/rentals/:id/return    rental:approver          main.go:1802
                        POST   /api/rentals-archive    rental:manager              main.go:1937

category_controller
                        GET    /api/categories         asset:viewer                main.go:1979
                        POST   /api/categories         asset:operator              main.go:1979
                        PUT    /api/categories/:id     asset:operator              main.go:2058
                        DELETE /api/categories/:id     asset:manager               main.go:2058

user_controller
                        GET    /api/users-list         認證                        main.go:1588
                        PUT    /api/users/:id          sys_admin                   main.go:1526
                        PUT    /api/users/:id/permissions  sys_admin               新增
                        POST   /api/change-password    認證                        main.go(推測)

profile_controller
                        GET    /api/profiles           mdm:operator                main.go:2083
                        POST   /api/profiles           mdm:operator                main.go:2083
                        DELETE /api/profiles/:id       mdm:operator                main.go:2175

notification_controller
                        GET    /api/notifications      rental:approver             新增
```

---

## 7. 前端 UI 改版

### 7.1 導覽結構 — 模組化側邊欄

```
側邊欄結構：

📊 Dashboard                    ← 全域（所有人可見）
─────────────────
📦 財產管理                      ← asset 模組群組
   ├── 財產清冊                  ← asset:viewer+
   ├── 資產分類                  ← asset:operator+
   └── 盤點作業                  ← asset:manager
─────────────────
📱 裝置管理                      ← mdm 模組群組
   ├── 裝置列表                  ← mdm:viewer+
   ├── 指令中心                  ← mdm:operator+
   ├── App 管理                  ← mdm:operator+
   ├── 描述檔                    ← mdm:operator+
   └── 即時事件                  ← mdm:viewer+
─────────────────
🔄 租借系統                      ← rental 模組群組
   ├── 租借管理                  ← rental:requester+
   └── 通知記錄                  ← rental:approver+
─────────────────
⚙️ 系統管理                      ← sys_admin only
   ├── 使用者管理
   ├── 權限設定
   └── 稽核日誌
```

### 7.2 前端路由結構

```tsx
// 模組化路由
<Route path="/dashboard" />

{/* 財產管理模組 */}
<Route path="/asset">
  <Route path="list" />          {/* 財產清冊 */}
  <Route path="categories" />    {/* 資產分類 */}
  <Route path="inventory" />     {/* 盤點作業 */}
</Route>

{/* MDM 裝置管理模組 */}
<Route path="/mdm">
  <Route path="devices" />       {/* 裝置列表 */}
  <Route path="devices/:udid" /> {/* 裝置詳情 */}
  <Route path="commands" />      {/* 指令中心 */}
  <Route path="apps" />          {/* App 管理 */}
  <Route path="profiles" />      {/* 描述檔 */}
  <Route path="events" />        {/* 即時事件 */}
</Route>

{/* 租借系統模組 */}
<Route path="/rental">
  <Route path="list" />          {/* 租借管理 */}
  <Route path="notifications" /> {/* 通知記錄 */}
</Route>

{/* 系統管理 */}
<Route path="/admin">
  <Route path="users" />         {/* 使用者管理 */}
  <Route path="permissions" />   {/* 權限設定 */}
  <Route path="audit" />         {/* 稽核日誌 */}
</Route>
```

### 7.3 權限控制（前端）

```tsx
// hooks/useModulePermission.ts
function useModulePermission(module: string): {
  hasAccess: boolean;
  level: 'none' | 'viewer' | 'requester' | 'operator' | 'approver' | 'manager';
  canView: boolean;
  canEdit: boolean;
  canManage: boolean;
}

// 導覽項目根據權限動態顯示/隱藏
// 頁面級 Guard：無權限時顯示 403 頁面
```

### 7.4 使用者權限管理 UI（sys_admin）

在「使用者管理」頁面新增權限設定 tab：

```
┌─────────────────────────────────────────┐
│ 使用者：王小明                            │
│ Email：wang@company.com                  │
├─────────────────────────────────────────┤
│ 模組權限                                 │
│                                          │
│ 財產管理   [▼ operator ]                 │
│ 裝置管理   [▼ viewer   ]                 │
│ 租借系統   [▼ requester]                 │
│                                          │
│            [儲存變更]                     │
└─────────────────────────────────────────┘
```

---

## 8. ISO 27001:2022 控制項對照

### 8.1 相關 Annex A 控制項

| 控制項 | 名稱 | 系統對應實作 |
|--------|------|-------------|
| **A.5.1** | 資訊安全政策 | 系統內建 RBAC + 模組權限，強制執行存取政策 |
| **A.5.2** | 資訊安全角色和責任 | `user_module_permissions` 明確定義每人在每模組的角色 |
| **A.5.3** | 職責分離 | 租借申請人 ≠ 核准人；模組權限互相獨立 |
| **A.5.9** | 資訊及其他相關資產的清冊 | 財產管理模組：完整的資產清冊 CRUD + 分類 |
| **A.5.10** | 資訊及其他相關資產的可接受使用 | 租借系統：使用目的記錄、審批流程、使用規範同意 |
| **A.5.11** | 資產歸還 | 租借系統：歸還流程 + 清點檢查表 + 通知 |
| **A.5.12** | 資訊的分類 | 資產分類（categories）樹狀結構 |
| **A.5.15** | 存取控制 | 模組級 RBAC，最小權限原則 |
| **A.5.16** | 身份管理 | JWT 認證 + 使用者帳號管理 |
| **A.5.17** | 鑑別資訊 | Argon2id 密碼雜湊 |
| **A.5.23** | 使用雲端服務之資訊安全 | Apple MDM/VPP/DEP 整合的安全控管（API Key 不外洩） |
| **A.5.33** | 記錄的保護 | `audit_logs` 只能 INSERT，通知記錄不可竄改 |
| **A.7.9** | 組織場所外部設備安全 | MDM 遠端管理：遺失模式、遠端清除、位置追蹤 |
| **A.7.10** | 儲存媒體 | 裝置清除（EraseDevice）功能 |
| **A.8.1** | 使用者端點裝置 | MDM 裝置管理全功能：描述檔、App 管控、安全設定 |
| **A.8.7** | 防範惡意軟體 | 透過 MDM 強制安裝安全設定描述檔 |
| **A.8.15** | 日誌記錄 | 稽核日誌（audit_logs）+ 通知記錄（notifications） |

### 8.2 稽核紀錄強化

現有 `audit_logs` 需補強以滿足 ISO 27001 A.5.33：

```sql
-- audit_logs 補強
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS module TEXT NOT NULL DEFAULT 'system';
-- module: 'system', 'asset', 'mdm', 'rental'

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS ip_address TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '';
```

記錄範圍擴充：

| 模組 | 需記錄的操作 |
|------|-------------|
| system | 登入/登出、密碼變更、權限變更 |
| asset | 資產新增/編輯/報廢/移撥、保管人變更 |
| mdm | MDM 指令執行、裝置同步、App 安裝/移除 |
| rental | 租借申請/核准/拒絕/借出/歸還 |

---

## 9. 實作里程碑

### Phase 0：Clean Architecture 重構（後端骨架）

> **目的**：先把 2318 行 main.go 拆乾淨，後續 Phase 才能在正確的位置加功能。
> **原則**：純重構，不改功能，前端零感知。

- [x] 建立 `internal/controller/` 目錄結構 + `helpers.go` + `router.go`
- [x] 建立新的 port 介面：`RentalRepository`, `AppRepository`, `CategoryRepository`
- [x] 建立對應的 postgres adapter 實作（從 main.go 提取 SQL）
- [x] 建立 service 層：`AssetService`, `RentalService`, `AppService`, `CategoryService`
- [x] 建立 controller 層：逐一把 main.go 的 handler 搬進對應 controller
  - [x] `auth_controller.go` — login/logout/me/register/setup
  - [x] `system_controller.go` — health/system-status/ws-config
  - [x] `device_controller.go` — devices/*/devices-list/devices-available/sync-device-info
  - [x] `asset_controller.go` — assets/*/device-status
  - [x] `app_controller.go` — managed-apps/*/device-apps/*/itunes-*/sync-device-apps
  - [x] `rental_controller.go` — rentals/*/rentals-archive
  - [x] `category_controller.go` — categories/*
  - [x] `user_controller.go` — users/*/users-list
  - [x] `profile_controller.go` — profiles/*
- [x] 瘦身 main.go 至 ~120 行（依賴注入 + 路由掛載）
- [ ] 全端點回歸測試（確保搬移後功能不變）

### Phase 1：模組權限 + DB Migration

- [x] 新增 migration `014_module_permissions.up.sql`
- [x] 新增 `user_module_permissions` 表
- [x] users 表新增 `email`, `system_role` 欄位
- [x] 新增 `notifications` 表
- [x] `audit_logs` 新增 `module`, `ip_address`, `user_agent` 欄位
- [x] 遷移現有使用者權限資料
- [x] 後端：新增 `PermissionRepo` + `AuthHelper` 模組權限中間件
- [x] 後端：controller 掛上 `RequireModule()` 權限檢查
- [x] 後端：`/api/me` 回傳模組權限
- [x] 前端：`useModulePermission` hook
- [x] 前端：更新 `authStore` 儲存模組權限

### Phase 2：UI 模組化導覽

- [x] 前端路由結構改為 `/asset/*`, `/mdm/*`, `/rental/*`, `/admin/*`
- [x] 側邊欄改為分組導覽（含模組標題分隔線）
- [x] 模組權限 Guard（無權限顯示 403）
- [x] 使用者管理頁面新增模組權限設定 UI

### Phase 3：Email 通知服務

- [x] 後端：新增 `smtp` adapter（實作 `port.EmailSender` 介面）
- [x] 後端：新增 `NotifyService`（service 層）+ HTML 模板
- [x] 後端：`RentalController` 整合 `NotifyService` 觸發通知（approve/reject/activate/return/create）
- [x] 後端：新增逾期檢查排程（每日執行，goroutine）
- [x] 後端：`notification_controller.go` — `/api/notifications`
- [x] 前端：通知記錄頁面

### Phase 4：財產管理模組強化

- [ ] 資產生命週期管理（狀態機：採購→啟用→報廢/移撥）
- [ ] 盤點作業功能
- [ ] 報表匯出強化

### Phase 5：稽核與合規

- [ ] 稽核日誌強化（module、IP、User-Agent）
- [ ] 稽核日誌查詢頁面支援模組篩選
- [ ] ISO 27001 控制項自評報告頁面（可選）

---

## 10. 擴充性設計

### 10.1 問題：認證系統寫死了

目前 `AuthService.Login()` 直接做：

```go
user, err := s.users.GetByUsername(ctx, req.Msg.Username)
if !verifyArgon2id(user.PasswordHash, req.Msg.Password) { ... }
```

未來要加 AD (LDAP)、FIDO (WebAuthn)、OIDC (Google/Azure SSO) 完全插不進去。

### 10.2 解法：AuthProvider 介面

將「驗證身份」這件事抽象化，支援多種認證方式並存：

```go
// internal/port/auth.go

// AuthProvider — 認證提供者介面
type AuthProvider interface {
    // Type 回傳認證方式識別碼
    Type() string  // "local", "ldap", "fido", "oidc"

    // Authenticate 驗證身份，成功回傳 user 資訊
    // 不同 provider 會從 credentials map 讀取不同欄位：
    //   local: {"username": "...", "password": "..."}
    //   ldap:  {"username": "...", "password": "..."}
    //   fido:  {"credential_id": "...", "assertion": "..."}
    //   oidc:  {"id_token": "...", "provider": "google"}
    Authenticate(ctx context.Context, credentials map[string]string) (*domain.User, error)

    // Available 回傳此 provider 是否已配置啟用
    Available() bool
}

// AuthProviderRegistry — 管理所有已註冊的 AuthProvider
type AuthProviderRegistry interface {
    Register(provider AuthProvider)
    Get(providerType string) (AuthProvider, bool)
    ListAvailable() []string
}
```

#### 各 Provider 的實作位置

```
internal/adapter/
├── auth/
│   ├── local_provider.go     # 現有：username + Argon2id 密碼
│   ├── ldap_provider.go      # 未來：AD / LDAP 認證
│   ├── fido_provider.go      # 未來：FIDO2 / WebAuthn
│   ├── oidc_provider.go      # 未來：Google / Azure SSO
│   └── registry.go           # AuthProviderRegistry 實作
```

#### Login 流程改造

```go
// internal/service/auth_service.go（改造後）

func (s *AuthService) Login(ctx context.Context, providerType string, credentials map[string]string) (*LoginResult, error) {
    // 1. 從 registry 取得對應的 provider
    provider, ok := s.providers.Get(providerType)
    if !ok {
        return nil, fmt.Errorf("unsupported auth provider: %s", providerType)
    }

    // 2. 呼叫 provider 驗證
    user, err := provider.Authenticate(ctx, credentials)
    if err != nil {
        return nil, err
    }

    // 3. 檢查帳號狀態
    if !user.IsActive {
        return nil, ErrAccountInactive
    }

    // 4. 載入模組權限、簽發 JWT（共用邏輯，不隨 provider 改變）
    permissions, _ := s.permissionRepo.GetByUserID(ctx, user.ID)
    token, err := s.generateToken(user, permissions)

    // 5. 寫入稽核日誌
    s.auditRepo.Create(ctx, &domain.AuditLog{
        Action: "login",
        Detail: fmt.Sprintf("provider=%s", providerType),
    })

    return &LoginResult{Token: token, User: user, Permissions: permissions}, nil
}
```

#### User domain 擴充

```go
// internal/domain/user.go（擴充）

type User struct {
    // ... 現有欄位 ...

    // 認證方式相關
    AuthProvider  string  // "local", "ldap", "fido", "oidc"
    ExternalID    string  // AD 的 DN、OIDC 的 sub claim、FIDO 的 credential ID
}
```

#### DB 擴充

```sql
-- users 表擴充
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '';

-- FIDO 認證需要額外的 credentials 表
CREATE TABLE IF NOT EXISTS fido_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   TEXT NOT NULL UNIQUE,    -- WebAuthn credential ID (base64url)
    public_key      BYTEA NOT NULL,          -- COSE public key
    sign_count      BIGINT NOT NULL DEFAULT 0,
    name            TEXT NOT NULL DEFAULT '', -- 使用者自訂的裝置名稱 (如 "YubiKey 5")
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ
);
CREATE INDEX idx_fido_user ON fido_credentials(user_id);
```

#### REST 端點擴充

```
auth_controller
    POST   /api/login                  # 改為接受 { "provider": "local", "username": "...", "password": "..." }
    POST   /api/login                  # 或     { "provider": "fido", "credential_id": "...", "assertion": "..." }
    POST   /api/login                  # 或     { "provider": "oidc", "id_token": "...", "provider": "google" }

    # FIDO 專用端點
    POST   /api/fido/register-begin    # 開始註冊（回傳 challenge）
    POST   /api/fido/register-finish   # 完成註冊（存 credential）
    POST   /api/fido/login-begin       # 開始驗證（回傳 challenge）
    POST   /api/fido/login-finish      # 完成驗證

    # OIDC 專用端點
    GET    /api/oidc/authorize/:provider   # 重導到 IdP 登入頁
    GET    /api/oidc/callback/:provider    # IdP 回調
```

#### 配置方式

```yaml
# 環境變數（main.go 依配置決定註冊哪些 provider）

# Local（預設啟用）
AUTH_LOCAL_ENABLED=true

# LDAP / AD
AUTH_LDAP_ENABLED=true
AUTH_LDAP_URL=ldap://ad.company.com:389
AUTH_LDAP_BASE_DN=DC=company,DC=com
AUTH_LDAP_BIND_DN=CN=svc-mdm,OU=ServiceAccounts,...
AUTH_LDAP_BIND_PASSWORD=xxx
AUTH_LDAP_USER_FILTER=(sAMAccountName={{username}})

# FIDO2 / WebAuthn
AUTH_FIDO_ENABLED=true
AUTH_FIDO_RP_ID=mdm.company.com          # Relying Party ID
AUTH_FIDO_RP_ORIGIN=https://mdm.company.com

# OIDC (Google / Azure)
AUTH_OIDC_ENABLED=true
AUTH_OIDC_PROVIDER=google
AUTH_OIDC_CLIENT_ID=xxx
AUTH_OIDC_CLIENT_SECRET=xxx
AUTH_OIDC_REDIRECT_URL=https://mdm.company.com/api/oidc/callback/google
```

#### main.go 中的註冊

```go
// cmd/server/main.go

// --- Auth Providers ---
authRegistry := auth.NewRegistry()
authRegistry.Register(auth.NewLocalProvider(userRepo))       // 永遠註冊

if cfg.LDAPEnabled {
    authRegistry.Register(auth.NewLDAPProvider(cfg.LDAP, userRepo))
}
if cfg.FIDOEnabled {
    authRegistry.Register(auth.NewFIDOProvider(cfg.FIDO, fidoRepo))
}
if cfg.OIDCEnabled {
    authRegistry.Register(auth.NewOIDCProvider(cfg.OIDC, userRepo))
}

authSvc := service.NewAuthService(authRegistry, permissionRepo, cfg.JWTSecret)
```

### 10.3 新增功能模組的擴充流程

假設未來要加「維修管理」模組：

```
步驟 1️⃣  DB Migration
    → 新增 repairs 表
    → INSERT module 名稱 'repair' 到權限系統

步驟 2️⃣  Domain 層
    → internal/domain/repair.go（新增 Repair 實體）

步驟 3️⃣  Port 層
    → internal/port/repository.go 新增 RepairRepository interface

步驟 4️⃣  Adapter 層
    → internal/adapter/postgres/repair_repo.go

步驟 5️⃣  Service 層
    → internal/service/repair_service.go（業務邏輯）

步驟 6️⃣  Controller 層
    → internal/controller/repair_controller.go
    → 實作 Registrable 介面，自動掛載路由

步驟 7️⃣  main.go
    → 加 3 行：建 repo、建 service、建 controller
    → 加進 controller.RegisterAll()

步驟 8️⃣  前端
    → 新增 /repair/* 路由 + 頁面
    → 導覽欄自動依模組權限顯示
```

**不需要改動的東西：**
- 權限系統（table-driven，`module='repair'` 自動生效）
- 認證系統（JWT + 模組權限已抽象化）
- 稽核系統（`audit_logs.module='repair'` 直接記）
- 前端權限 hook（`useModulePermission('repair')` 自動生效）

### 10.4 模組 Registry（可選，進階）

如果模組多到需要動態發現，可以加一層 registry：

```go
// internal/module/registry.go

type Module struct {
    Name        string            // "asset", "mdm", "rental", "repair"
    Permissions []string          // 該模組支援的權限層級
    Controller  controller.Registrable
}

type ModuleRegistry struct {
    modules map[string]*Module
}

func (r *ModuleRegistry) Register(m *Module) { r.modules[m.Name] = m }
func (r *ModuleRegistry) List() []*Module    { /* 回傳所有已註冊模組 */ }
```

好處：
- `/api/modules` 端點可回傳所有可用模組（前端動態生成導覽）
- 權限設定 UI 自動列出所有模組 + 對應的權限層級
- 新增模組時前後端都不需要 hardcode 模組名稱

> **但目前階段不建議實作**——3 個模組直接寫比抽象 registry 更清楚。
> 等模組超過 5 個再考慮。

### 10.5 擴充性總覽

| 擴充場景 | 需改動的層 | 不需改動的層 |
|---------|-----------|------------|
| 加認證方式 (AD/FIDO/OIDC) | `adapter/auth/` 新 provider + `main.go` 註冊 | Service、Controller、前端 auth 流程 |
| 加功能模組 (維修/採購) | 全層新增（domain→port→adapter→service→controller） | 認證、權限系統、稽核、通知 |
| 加通知管道 (Slack/Teams) | `adapter/` 新 sender + `NotifyService` 加 channel | Controller、前端 |
| 加外部整合 (SIEM/CMDB) | `adapter/` 新 client | 現有功能不受影響 |

---

## 11. 技術決策摘要

| 決策 | 選擇 | 理由 |
|------|------|------|
| 模組拆分策略 | 單體內模組化（非微服務） | 團隊規模小、部署簡單、避免過度工程 |
| 權限模型 | 全域角色 + 模組級權限 | 靈活度高，滿足職責分離需求 |
| Email 方案 | Go net/smtp + HTML template | 輕量、無外部依賴、易於部署 |
| 通知記錄 | 寫入 DB | 滿足稽核可追溯要求 |
| 路由結構 | `/module/page` 二層路由 | 清晰的模組邊界，URL 即可看出所屬模組 |
| 舊 role 遷移 | 保留 `role` 欄位，新增 `system_role` | 向下相容，漸進遷移 |
