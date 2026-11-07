import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { currentSection } from './sections';

// Narrow each docs page's sidebar to its own section. Get started pages read
// as a short sequence, so they drop the sidebar and table of contents and
// show numbered steps instead (see components/PageTitle.astro).
export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  const section = currentSection(route.sidebar);
  if (!section) return;
  route.sidebar = route.sidebar.filter((e) => e.type === 'group' && section.groups.includes(e.label));
  if (section.label === 'Get started') {
    route.hasSidebar = false;
    route.toc = undefined;
  }
});
