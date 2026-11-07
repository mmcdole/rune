import type { StarlightRouteData } from '@astrojs/starlight/route-data';

type SidebarEntry = StarlightRouteData['sidebar'][number];

export interface Section {
  label: string;
  /** Landing page of the section, relative to the site base. */
  href: string;
  /** Labels of the top-level sidebar groups (astro.config.mjs) it owns. */
  groups: string[];
}

// The site's top-level sections. The header nav lists them, and each docs
// page shows only its own section's sidebar groups.
export const sections: Section[] = [
  { label: 'Get started', href: 'getting-started/installation/', groups: ['Get started'] },
  { label: 'Guide', href: 'scripting/model/', groups: ['Scripting', 'Interface', 'Recipes'] },
  { label: 'Reference', href: 'reference/api/', groups: ['Lua API', 'Reference'] },
];

const containsCurrent = (entry: SidebarEntry): boolean =>
  entry.type === 'link' ? entry.isCurrent : entry.entries.some(containsCurrent);

/** The section whose sidebar groups contain the current page, if any. */
export function currentSection(sidebar: SidebarEntry[]): Section | undefined {
  const group = sidebar.find((e) => e.type === 'group' && containsCurrent(e));
  return group && sections.find((s) => s.groups.includes(group.label));
}

/** The pages of a top-level sidebar group, in sidebar order. */
export function groupLinks(sidebar: SidebarEntry[], label: string) {
  const group = sidebar.find((e) => e.type === 'group' && e.label === label);
  if (!group || group.type !== 'group') return [];
  return group.entries.flatMap((e) => (e.type === 'link' ? [e] : []));
}
