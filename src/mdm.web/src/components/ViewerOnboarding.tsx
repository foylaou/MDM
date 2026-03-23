import { useLocation } from 'react-router-dom';
import { useAuthStore } from '../stores/authStore';
import { useDriverOnboarding, type Role, type OnboardingContext, type StepDef } from '../hooks/useDriverOnboarding';
import { tourStepsByScope } from '../tours/viewerTours';
import { HelpCircle } from 'lucide-react';

function resolveScope(pathname: string): string {
  if (/^\/devices\/.+/.test(pathname)) return 'device-detail';
  return pathname.replace('/', '') || 'dashboard';
}

export function ViewerOnboarding() {
  const { user } = useAuthStore();
  const { pathname } = useLocation();

  const scope = resolveScope(pathname);
  const steps = tourStepsByScope[scope];

  if (!steps?.length || user?.role !== 'viewer') return null;

  const ctx: OnboardingContext = {
    role: (user?.role as Role) || null,
    permissions: [],
    pathname,
  };

  // key={scope} forces remount on page change, so the tour auto-starts per page
  return <TourRunner key={scope} steps={steps} ctx={ctx} scope={scope} />;
}

function TourRunner({ steps, ctx, scope }: { steps: StepDef[]; ctx: OnboardingContext; scope: string }) {
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
