import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
  site: 'https://mmcdole.github.io',
  base: '/rune',
  integrations: [
    starlight({
      title: 'ᚱune',
      description: 'A fast, modern MUD client with careful terminal ergonomics and a Lua API that goes all the way down.',
      social: [
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
            { label: 'Aliases', slug: 'scripting/aliases' },
            { label: 'Triggers', slug: 'scripting/triggers' },
            { label: 'Timers', slug: 'scripting/timers' },
            { label: 'Hooks & Events', slug: 'scripting/hooks' },
            { label: 'Key Bindings', slug: 'scripting/keybindings' },
            { label: 'Slash Commands', slug: 'scripting/commands' },
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
            { label: 'Slash Commands', slug: 'reference/slash-commands' },
            { label: 'Hook Events', slug: 'reference/hook-events' },
            { label: 'Protocols', slug: 'reference/protocols' },
          ],
        },
      ],
    }),
  ],
});
