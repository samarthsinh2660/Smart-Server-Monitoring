"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

export function RefreshTicker() {
  const router = useRouter();
  useEffect(() => {
    const timer = window.setInterval(() => router.refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [router]);
  return <span className="refresh-note">Refreshes every 5 seconds</span>;
}
