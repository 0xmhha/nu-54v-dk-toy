import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Operations assets are served by the same mTLS origin as opsd.
export default defineConfig({ base: "/ops/", plugins: [react()], resolve: { dedupe: ["react", "react-dom"] } });
