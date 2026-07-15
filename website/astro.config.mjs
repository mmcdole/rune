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
      title: 'ᚱune',
      description: 'A fast, modern MUD client with careful terminal ergonomics and a Lua API that goes all the way down.',
      social: [
        { icon: 'discord', label: 'Discord', href: 'https://discord.gg/gNZkrJ2jHe' },
        { icon: 'github', label: 'GitHub', href: 'https://github.com/mmcdole/rune' },
      ],
      customCss: ['./src/styles/custom.css'],
      components: {
        ThemeProvider: './src/components/ForceDark.astro',
        ThemeSelect: './src/components/NoThemeSelect.astro',
        SocialIcons: './src/components/HeaderLinks.astro',
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Your First Session', slug: 'getting-started/first-session' },
            { label: 'Scripting Basics', slug: 'getting-started/scripting-basics' },
            { label: 'Migrating from Other Clients', slug: 'getting-started/migrating' },
          ],
        },
        {
          label: 'Scripting',
          items: [
            { label: 'The Scripting Model', slug: 'scripting/model' },
            { label: 'Triggers', slug: 'scripting/triggers' },
            { label: 'Aliases', slug: 'scripting/aliases' },
            { label: 'Timers', slug: 'scripting/timers' },
            { label: 'Hooks & Events', slug: 'scripting/hooks' },
            { label: 'Key Bindings', slug: 'scripting/keybindings' },
            { label: 'Custom Commands', slug: 'scripting/commands' },
            { label: 'Groups', slug: 'scripting/groups' },
            { label: 'GMCP', slug: 'scripting/gmcp' },
            { label: 'Storage & Worlds', slug: 'scripting/storage' },
            { label: 'Logging', slug: 'scripting/logging' },
          ],
        },
        {
          label: 'Interface',
          items: [
            { label: 'Input & History', slug: 'interface/input' },
            { label: 'Layout & UI', slug: 'interface/layout' },
            { label: 'Bars', slug: 'interface/bars' },
            { label: 'Panes', slug: 'interface/panes' },
            { label: 'Pickers', slug: 'interface/pickers' },
          ],
        },
        {
          label: 'Cookbook',
          items: [
            { label: 'Quake-Style Chat Console', slug: 'cookbook/quake-console' },
            { label: 'Forward Tells to Telegram', slug: 'cookbook/telegram' },
            { label: 'HP Bar from GMCP', slug: 'cookbook/hp-bar' },
            { label: 'Highlight & Gag Sets', slug: 'cookbook/highlights' },
            { label: 'Auto-Login with Worlds', slug: 'cookbook/autologin' },
          ],
        },
        {
          label: 'Reference',
          items: [
            {
              label: 'Lua API',
              collapsed: true,
              items: [
                { label: 'Overview', slug: 'reference/api' },
                { label: 'Core', slug: 'reference/api/core' },
                { label: 'State & Lines', slug: 'reference/api/state-lines' },
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
                { label: 'rune.input', slug: 'reference/api/input' },
                { label: 'Storage', slug: 'reference/api/storage' },
                { label: 'rune.log', slug: 'reference/api/log' },
                { label: 'rune.ui', slug: 'reference/api/ui' },
                { label: 'rune.ui.picker', slug: 'reference/api/picker' },
                { label: 'rune.pane', slug: 'reference/api/pane' },
              ],
            },
            { label: 'Slash Commands', slug: 'reference/slash-commands' },
            { label: 'Protocols', slug: 'reference/protocols' },
          ],
        },
      ],
    }),
  ],
});
