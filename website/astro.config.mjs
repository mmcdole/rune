import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
  site: 'https://runemud.com',
  redirects: {
    '/reference/hook-events/': '/reference/api/hooks/',
  },
  integrations: [
    starlight({
      title: 'Rune',
      description: 'A terminal MUD client with Lua scripting, configurable layouts, command history, and server-output search.',
      social: [
        { icon: 'discord', label: 'Discord', href: 'https://discord.gg/gNZkrJ2jHe' },
        { icon: 'github', label: 'GitHub', href: 'https://github.com/mmcdole/rune' },
      ],
      customCss: [
        '@fontsource/ibm-plex-sans/400.css',
        '@fontsource/ibm-plex-sans/500.css',
        '@fontsource/ibm-plex-sans/600.css',
        '@fontsource/ibm-plex-mono/400.css',
        '@fontsource/ibm-plex-mono/600.css',
        './src/styles/custom.css',
      ],
      routeMiddleware: './src/routeData.ts',
      components: {
        ThemeProvider: './src/components/ForceDark.astro',
        ThemeSelect: './src/components/NoThemeSelect.astro',
        SiteTitle: './src/components/SiteTitle.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      // Top-level group labels are matched by src/sections.ts, which assigns
      // each group to a header section.
      sidebar: [
        {
          label: 'Get started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Your first session', slug: 'getting-started/first-session' },
            { label: 'Scripting basics', slug: 'getting-started/scripting-basics' },
            { label: 'Coming from another client', slug: 'getting-started/migrating' },
          ],
        },
        {
          label: 'Scripting',
          items: [
            { label: 'The scripting model', slug: 'scripting/model' },
            { label: 'Triggers', slug: 'scripting/triggers' },
            { label: 'Aliases', slug: 'scripting/aliases' },
            { label: 'Timers', slug: 'scripting/timers' },
            { label: 'Hooks and events', slug: 'scripting/hooks' },
            { label: 'Key bindings', slug: 'scripting/keybindings' },
            { label: 'Custom commands', slug: 'scripting/commands' },
            { label: 'Groups', slug: 'scripting/groups' },
            { label: 'GMCP', slug: 'scripting/gmcp' },
            { label: 'Storage and worlds', slug: 'scripting/storage' },
            { label: 'Logging', slug: 'scripting/logging' },
          ],
        },
        {
          label: 'Interface',
          items: [
            { label: 'Input and history', slug: 'interface/input' },
            { label: 'Layout', slug: 'interface/layout' },
            { label: 'Bars', slug: 'interface/bars' },
            { label: 'Panes', slug: 'interface/panes' },
            { label: 'Pickers', slug: 'interface/pickers' },
          ],
        },
        {
          label: 'Recipes',
          items: [
            { label: 'Quake-style chat console', slug: 'cookbook/quake-console' },
            { label: 'Forward tells to Telegram', slug: 'cookbook/telegram' },
            { label: 'HP bar from GMCP', slug: 'cookbook/hp-bar' },
            { label: 'Highlight and gag sets', slug: 'cookbook/highlights' },
            { label: 'Auto-login with worlds', slug: 'cookbook/autologin' },
          ],
        },
        {
          label: 'Lua API',
          items: [
            { label: 'Overview', slug: 'reference/api' },
            { label: 'Core', slug: 'reference/api/core' },
            { label: 'State and lines', slug: 'reference/api/state-lines' },
            { label: 'rune.style', slug: 'reference/api/style' },
            { label: 'rune.regex', slug: 'reference/api/regex' },
            { label: 'rune.trigger', slug: 'reference/api/trigger' },
            { label: 'rune.alias', slug: 'reference/api/alias' },
            { label: 'rune.timer', slug: 'reference/api/timer' },
            { label: 'rune.hooks', slug: 'reference/api/hooks' },
            { label: 'rune.bind', slug: 'reference/api/bind' },
            { label: 'rune.command', slug: 'reference/api/command' },
            { label: 'rune.group', slug: 'reference/api/group' },
            { label: 'rune.gmcp', slug: 'reference/api/gmcp' },
            { label: 'rune.http', slug: 'reference/api/http' },
            { label: 'rune.json', slug: 'reference/api/json' },
            { label: 'rune.input', slug: 'reference/api/input' },
            { label: 'Storage', slug: 'reference/api/storage' },
            { label: 'rune.log', slug: 'reference/api/log' },
            { label: 'rune.ui', slug: 'reference/api/ui' },
            { label: 'rune.ui.picker', slug: 'reference/api/picker' },
            { label: 'rune.pane', slug: 'reference/api/pane' },
            { label: 'rune.clipboard', slug: 'reference/api/clipboard' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'Slash commands', slug: 'reference/slash-commands' },
            { label: 'Protocols', slug: 'reference/protocols' },
          ],
        },
      ],
    }),
  ],
});
