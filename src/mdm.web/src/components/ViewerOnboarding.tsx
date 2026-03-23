import { useLocation } from 'react-router-dom';
import { useAuthStore } from '../stores/authStore';
import { useDriverOnboarding, type Role, type OnboardingContext } from '../hooks/useDriverOnboarding';
import { tourStepsByScope } from '../tours/viewerTours';
import { HelpCircle } from 'lucide-react';

const EMPTY_STEPS: [] = [];

function resolveScope(pathname: string): string {
  // /devices/:udid → device-detail
  if (/^\/devices\/.+/.test(pathname)) return 'device-detail';
  return pathname.replace('/', '') || 'dashboard';
}

export function ViewerOnboarding() {
  const { user } = useAuthStore();
  const { pathname } = useLocation();

  const scope = resolveScope(pathname);
  const steps = tourStepsByScope[scope] || EMPTY_STEPS;

  const ctx: OnboardingContext = {
    role: (user?.role as Role) || null,
    permissions: [],
    pathname,
  };

  const { start, resetSeen } = useDriverOnboarding(steps, {
    ctx,
    scope,
    autoStartInProd: true,
    devAutoStart: true,
    waitMs: 500,
  });

  const handleRestart = () => {
    resetSeen();
    setTimeout(() => start(true), 100);
  };

  if (!steps.length || user?.role !== 'viewer') return null;

  return (
    <div className="tooltip tooltip-left" data-tip="操作導覽">
      <button
        onClick={handleRestart}
        className="btn btn-circle btn-primary btn-lg fixed bottom-6 right-6 z-50 shadow-lg"
      >
        <HelpCircle size={28} />
      </button>
    </div>
  );
}
