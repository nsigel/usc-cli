import type { APIRoute } from "astro";
import { docs, site } from "../docs";

export const GET: APIRoute = () => {
  const urls = ["/", ...docs.map((doc) => doc.href)].map((href) => `  <url><loc>${site}${href}</loc></url>`);
  return new Response(
    `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls.join("\n")}\n</urlset>\n`,
  );
};
