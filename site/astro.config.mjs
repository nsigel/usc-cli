import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://usc.sh",
  trailingSlash: "always",
  markdown: {
    shikiConfig: { theme: "min-light" },
  },
});
