import { useAuthStore } from "../stores/authStore";

const levelOrder: Record<string, number> = {
  none: 0,
  viewer: 1,
  requester: 2,
  operator: 3,
  approver: 4,
  manager: 5,
};

export type PermissionLevel = "none" | "viewer" | "requester" | "operator" | "approver" | "manager";

export function useModulePermission(module: string) {
  const { user, modulePermissions } = useAuthStore();

  const isSysAdmin = user?.system_role === "sys_admin" || user?.role === "admin";
  const raw = modulePermissions[module] || "none";
  const level: PermissionLevel = isSysAdmin ? "manager" : (raw as PermissionLevel);
  const rank = levelOrder[level] ?? 0;

  return {
    level,
    hasAccess: rank > 0,
    canView: rank >= levelOrder.viewer,
    canRequest: rank >= levelOrder.requester,
    canOperate: rank >= levelOrder.operator,
    canApprove: rank >= levelOrder.approver,
    canManage: rank >= levelOrder.manager,
    isSysAdmin,
  };
}
