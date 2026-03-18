import axios from "axios";

const baseURL = import.meta.env.DEV ? "" : window.location.origin;

const apiClient = axios.create({
  baseURL,
  withCredentials: true, // always send HttpOnly cookies
  headers: { "Content-Type": "application/json" },
});

// Response interceptor — auto redirect to login on 401
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Only redirect if not already on login/setup page
      const path = window.location.pathname;
      if (path !== "/login" && path !== "/setup") {
        window.location.href = "/login";
      }
    }
    return Promise.reject(error);
  }
);

export default apiClient;
