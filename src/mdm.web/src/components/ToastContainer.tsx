import { useEventStore } from "../stores/eventStore";
import { X, CheckCircle, AlertCircle, Info } from "lucide-react";

export function ToastContainer() {
  const { toasts, dismissToast } = useEventStore();

  if (toasts.length === 0) return null;

  const icons = {
    success: <CheckCircle size={16} />,
    error: <AlertCircle size={16} />,
    info: <Info size={16} />,
  };

  const alertClass = {
    success: "alert-success",
    error: "alert-error",
    info: "alert-info",
  };

  return (
    <div className="toast toast-end toast-bottom z-50">
      {toasts.slice(-5).map((toast) => (
        <div key={toast.id} className={`alert ${alertClass[toast.type]} shadow-lg py-2 px-4 min-w-64 max-w-sm`}>
          {icons[toast.type]}
          <span className="text-sm flex-1 truncate">{toast.message}</span>
          <button onClick={() => dismissToast(toast.id)} className="btn btn-ghost btn-xs btn-circle">
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
