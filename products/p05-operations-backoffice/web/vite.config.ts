import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// P05 back-office UI ([N20]). Talks to opsd; screens arrive next cycle.
export default defineConfig({ plugins: [react()] });
