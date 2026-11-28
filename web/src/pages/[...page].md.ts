import type { APIRoute, GetStaticPaths } from "astro";
import { docs, frontMatter, markdownPath, overview, site, homeDescription } from "../docs";

// Each page has a Markdown version at <page>/index.md. The site server
// returns it for requests that accept text/markdown.
export const getStaticPaths = (() => [
  {
    params: { page: "index" },
    props: { body: `${frontMatter("usc.sh", homeDescription, `${site}/`)}\n\n${overview()}` },
  },
  ...docs.map((doc) => ({
    params: { page: markdownPath(doc.href).slice(1, -".md".length) },
    props: {
      body: `${frontMatter(doc.title, doc.description, `${site}${doc.href}`)}\n\n# ${doc.title}\n\n${doc.body}\n`,
    },
  })),
]) satisfies GetStaticPaths;

export const GET: APIRoute = ({ props }) => new Response(props.body);
