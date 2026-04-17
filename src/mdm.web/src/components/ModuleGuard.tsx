import { useModulePermission, type PermissionLevel } from "../hooks/useModulePermission";
import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";

interface ModuleGuardProps {
  module: string;
  minLevel?: PermissionLevel;
  children: React.ReactNode;
}

export function ModuleGuard({ module, minLevel = "viewer", children }: ModuleGuardProps) {
  const { t } = useTranslation();
  const perm = useModulePermission(module);

  const levelOrder: Record<string, number> = {
    none: 0, viewer: 1, requester: 2, operator: 3, approver: 4, manager: 5,
  };

  const hasAccess = (levelOrder[perm.level] ?? 0) >= (levelOrder[minLevel] ?? 0);

  if (!hasAccess) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center text-center gap-4">
        <ShieldAlert size={64} className="text-error opacity-50" />
        <h2 className="text-2xl font-bold">{t("nav.forbidden")}</h2>
        <p className="text-base-content/60 max-w-md">{t("nav.forbidden_desc")}</p>
      </div>
    );
  }

  return <>{children}</>;
}
