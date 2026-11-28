import type { APIRoute } from "astro";
import { overview } from "../docs";

export const GET: APIRoute = () => new Response(overview());
