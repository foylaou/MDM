'use client';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { driver } from 'driver.js';

// ====== 你的角色與權限 ======
export type Role = 'admin' | 'operator' | 'viewer' | null;
export type Permission = string;

export type OnboardingContext = {
    role: Role;
    permissions: Permission[];
    pathname: string;
};

// ====== Step 定義（可加條件）======
export type Guard = (ctx: OnboardingContext) => boolean;
export type StepDef = {
    selector: string;
    title: string;
    description?: string;
    guard?: Guard; // 回傳 true 才會顯示
};

export const makeStep = (
    selector: string,
    title: string,
    description?: string,
    guard?: Guard
): StepDef => ({ selector, title, description, guard });

// 常用 guard：依角色 & 權限
export const whenRole =
    (...roles: Role[]) =>
        (ctx: OnboardingContext) =>
            !!ctx.role && roles.includes(ctx.role);

export const whenPermAny =
    (...perms: Permission[]) =>
        (ctx: OnboardingContext) =>
            perms.some((p) => ctx.permissions?.includes(p));

export const and =
    (...guards: Guard[]) =>
        (ctx: OnboardingContext) =>
            guards.every((g) => g(ctx));

export const or =
    (...guards: Guard[]) =>
        (ctx: OnboardingContext) =>
            guards.some((g) => g(ctx));

export type UseDriverOnboardingOptions = {
    version?: string;             // e.g. '1.0.0'（預設取 NEXT_PUBLIC_ONBOARDING_VERSION）
    storageKey?: string;          // e.g. 'onboarding_seen'
    scope?: string;               // e.g. 'login' / 'home' / 'kpi-dashboard'
    autoStartInProd?: boolean;    // 預設 true
    devAutoStart?: boolean;       // 預設 false（Dev 手動）
    minShownMs?: number;          // 預設 400
    waitMs?: number;              // 預設 0（用 rAF 也可）
    i18n?: { next?: string; prev?: string; done?: string };
    chainNext?: string;           // ⚡️跨頁：導覽完成後導向下一頁（e.g. '/kpi/dashboard'）
    forceChain?: boolean;         // ⚡️忽略下一頁已 seen，仍強制顯示（預設 false）
    ctx: OnboardingContext;       // ⚡️由呼叫端提供（角色/權限/路由）
};

export function useDriverOnboarding(stepDefs: StepDef[], opts: UseDriverOnboardingOptions) {
    const {
        version = '1.0.0',
        storageKey = 'onboarding_seen',
        scope = 'page',
        autoStartInProd = true,
        devAutoStart = false,
        minShownMs = 400,
        waitMs = 0,
        i18n = { next: '下一步', prev: '上一步', done: '完成' },
        chainNext,
        forceChain = false,
        ctx,
    } = opts;

    // 版本化 + 分頁 key
    const KEY = `${storageKey}_${version}_${scope}`;
    const NEXT_KEY = `${storageKey}_${version}__pending_next`; // 暫存下一頁 scope（session）
    const initRef = useRef(false);

    const guardedSteps = useMemo(() => {
        return stepDefs.filter((s) => !s.guard || s.guard(ctx));
    }, [stepDefs, ctx]);

    const hasSeen = useCallback(() => !!localStorage.getItem(KEY), [KEY]);
    const setSeen = useCallback(() => localStorage.setItem(KEY, '1'), [KEY]);
    const resetSeen = useCallback(() => localStorage.removeItem(KEY), [KEY]);

    const start = useCallback((forced = false) => {
        // 1) 此刻才檢查 DOM 是否存在（避免切頁回來 steps 被算成空陣列）
        const present = guardedSteps.filter((s) => {
            try { return !!document.querySelector(s.selector); } catch { return false; }
        });
        if (!present.length) {
            if (forced) setTimeout(() => start(true), 600);
            return;
        }

        // 2) 轉成 Driver.js 需要的 steps 形狀
        const driverSteps = present.map((s) => ({
            element: s.selector,
            popover: { title: s.title, description: s.description },
        }));

        const startedAt = Date.now();
        const drv = driver({
            showProgress: true,
            nextBtnText: i18n.next,
            prevBtnText: i18n.prev,
            // 如果有跨頁，把「完成」文字改成「下一頁」
            doneBtnText: chainNext ? '下一頁' : i18n.done,
            steps: driverSteps,

            // 自訂按鈕行為：最後一步 → 導頁；否則照常下一步
            onNextClick: () => {
                if (chainNext && drv.isLastStep()) {
                    try {
                        sessionStorage.setItem(NEXT_KEY, JSON.stringify({ to: chainNext, force: !!forceChain }));
                    } catch {}
                    drv.destroy();                         // 關閉導覽
                    window.location.assign(chainNext);     // 換頁
                } else {
                    drv.moveNext();                        // 照常下一步
                }
            },
            onPrevClick: () => drv.movePrevious(),
            // onCloseClick: () => drv.destroy(),     // 需要時可覆寫 close 行為
        });

        try { drv.drive(); } catch { return; }

        const onDestroyed = () => {
            const shown = Date.now() - startedAt;
            if (shown > minShownMs) setSeen();
            else initRef.current = false;
        };
        window.addEventListener('driver:destroyed', onDestroyed, { once: true });
    }, [guardedSteps, chainNext, i18n.next, i18n.prev, i18n.done, minShownMs, setSeen, forceChain]);

    // Dev 工具
    useEffect(() => {
        (window as any).__tour = {
            key: KEY,
            start: (force = true) => { resetSeen(); start(force); },
            reset: resetSeen,
            seen: hasSeen,
        };
    }, [KEY, start, resetSeen, hasSeen]);

    // 自動啟動（Dev 預設手動 / Prod 自動一次）
    useEffect(() => {
        if (initRef.current) return;
        initRef.current = true;

        const isDev = import.meta.env.DEV;
        const shouldAuto = isDev ? devAutoStart : autoStartInProd;

        if (!shouldAuto && !sessionStorage.getItem(NEXT_KEY)) return;

        // 1) 跨頁續接：若有 pending_next，則在該頁強制啟動
        const pending = sessionStorage.getItem(NEXT_KEY);
        if (pending) {
            try {
                const { to, force } = JSON.parse(pending);
                sessionStorage.removeItem(NEXT_KEY);
                // 若路由吻合，強制啟動；不吻合則忽略
                if (to && ctx.pathname.startsWith(to)) {
                    start(!!force);
                    return;
                }
            } catch { sessionStorage.removeItem(NEXT_KEY); }
        }

        // 2) 非續接：依是否看過決定
        if (hasSeen()) return;

        const run = () => start(false);
        if (waitMs > 0) setTimeout(run, waitMs);
        else requestAnimationFrame(() => requestAnimationFrame(run));
    }, [autoStartInProd, devAutoStart, hasSeen, start, waitMs, ctx.pathname]);

    return { start, resetSeen, hasSeen, key: KEY };
}

// 元件包裝
export function OnboardingTour(props: {
    steps: StepDef[];
    options: UseDriverOnboardingOptions;
}) {
    useDriverOnboarding(props.steps, props.options);
    return null;
}
