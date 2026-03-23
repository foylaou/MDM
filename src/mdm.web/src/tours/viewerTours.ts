import { makeStep, type StepDef } from '../hooks/useDriverOnboarding';

// ====== Dashboard ======
export const dashboardSteps: StepDef[] = [
  makeStep('[data-tour="nav-sidebar"]',  '歡迎使用 MDM 裝置管理系統', '左側導航欄可以切換頁面，你主要會用到「裝置管理」和「租借管理」。'),
  makeStep('[data-tour="stats-cards"]',  '系統總覽', '這裡顯示裝置總數、在線數量、借出數量等即時資訊。'),
  makeStep('[data-tour="quick-actions"]', '快速操作', '點擊可快速跳轉到裝置列表或租借管理。'),
];

// ====== Rentals（重點頁面）======
export const rentalsSteps: StepDef[] = [
  makeStep('[data-tour="rental-workflow"]', '租借流程說明',
    '租借有四個階段：\n' +
    '1. 提交申請（待核准）\n' +
    '2. 保管人或管理員審核（已核准）\n' +
    '3. 管理員確認借出（借出中）\n' +
    '4. 歸還裝置（已歸還）\n\n' +
    '提交後請通知裝置保管人進行審核。'),
  makeStep('[data-tour="rental-create"]', '第一步：點此申請借用',
    '點擊「新增租借」開始申請。\n\n' +
    '在選擇裝置時可以：\n' +
    '• 用「狀態篩選」找到「可用」的裝置\n' +
    '• 用「分類篩選」依類型挑選（如 iPad mini、iPad Air）\n' +
    '• 用搜尋欄輸入裝置名稱或序號\n' +
    '• 用「快選」一次選取多台\n\n' +
    '選好後填寫用途與預計歸還日期，提交申請。'),
  makeStep('[data-tour="rental-filter"]', '篩選租借記錄',
    '依狀態篩選你的租借紀錄，例如查看「借出中」確認目前借了哪些裝置。'),
  makeStep('[data-tour="rental-table"]', '追蹤你的租借',
    '所有申請都會列在這裡。\n\n' +
    '核准通過後，借出的裝置會出現在「裝置管理」頁面中，你可以在那裡操控裝置。'),
];

// ====== Devices ======
export const devicesSteps: StepDef[] = [
  makeStep('[data-tour="device-table"]', '你的借用裝置',
    '這裡列出你目前借用中的裝置。\n點擊任一裝置可進入詳細頁面，進行遠端操控。'),
  makeStep('[data-tour="device-search"]', '搜尋裝置', '輸入裝置名稱或序號快速定位。'),
];

// ====== Device Detail ======
export const deviceDetailSteps: StepDef[] = [
  makeStep('[data-tour="device-actions"]', '裝置操控',
    '這裡是你可以使用的裝置操作：\n\n' +
    '• 鎖定裝置 — 遠端鎖定螢幕\n' +
    '• 啟用遺失模式 — 裝置遺失時鎖定並顯示訊息\n' +
    '• 定位 / 播放聲音 — 啟用遺失模式後可用\n' +
    '• 更新 App — 更新裝置上已安裝的應用\n\n' +
    '如需安裝新 App 或移除 App，請聯繫管理員。'),
  makeStep('[data-tour="device-tabs"]', '裝置資訊分頁',
    '切換分頁查看裝置詳細資訊，包含已裝 App、描述檔、安全性等。\n點擊「同步」可取得最新資料。'),
];

// Scope → steps mapping
export const tourStepsByScope: Record<string, StepDef[]> = {
  dashboard: dashboardSteps,
  devices: devicesSteps,
  rentals: rentalsSteps,
  'device-detail': deviceDetailSteps,
};
