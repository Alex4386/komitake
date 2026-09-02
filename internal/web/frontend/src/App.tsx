import { Navigate, Route, Routes } from "react-router-dom";
import { KartDashboardPage } from "@/pages/KartDashboardPage";
import { KartFullscreenPage } from "@/pages/KartFullscreenPage";
import { LandingPage } from "@/pages/LandingPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { SettingsPage } from "@/pages/SettingsPage";

export default function App() {
  return (
    <Routes>
      <Route index element={<LandingPage />} />
      <Route path="karts/:slug" element={<KartDashboardPage />} />
      <Route path="karts/:slug/fullscreen" element={<KartFullscreenPage />} />
      <Route path="settings" element={<SettingsPage />} />
      <Route path="home" element={<Navigate to="/" replace />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
