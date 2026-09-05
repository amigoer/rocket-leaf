import type { Content } from './types';

export const en: Content = {
  htmlLang: 'en',
  meta: {
    title: 'MQ Studio — One interface for every message queue',
    description:
      'MQ Studio is a local-first desktop client for message queues. RocketMQ, RabbitMQ, Kafka, Pulsar, Redis Stream, MQTT, NATS, ActiveMQ, and NSQ share one interface and one workflow, with no web console to deploy or keep alive. macOS, Windows, and Linux. Apache-2.0.',
    ogAlt: 'The MQ Studio cluster overview',
  },
  banner: {
    text: 'NSQ has landed — topics and channels, the discovery tier, and who is consuming',
    linkLabel: 'Changelog',
    dismiss: 'Dismiss announcement',
  },
  nav: {
    features: 'Features',
    modules: 'Product tour',
    roadmap: 'Roadmap',
    docs: 'Docs',
    changelog: 'Changelog',
    github: 'GitHub',
    download: 'Download',
    moreDownloads: 'More download options',
    skipToContent: 'Skip to main content',
    home: 'Home',
    breadcrumb: 'Breadcrumb',
    languageLabel: '中文',
    menu: 'Open menu',
    theme: 'Toggle theme',
  },
  hero: {
    badgeSuffix: 'is out · Apache-2.0',
    title: 'One interface for every message queue',
    subtitle:
      'MQ Studio is a local-first desktop client for message queues — RocketMQ, RabbitMQ, Kafka, Pulsar, Redis Stream, MQTT, NATS, ActiveMQ, and NSQ behind the same pages and the same workflow, with no web console to deploy or keep alive.',
    downloadFallback: 'Download MQ Studio',
    downloadFor: (platform: string) => `Download for ${platform}`,
    installGuide: 'Install guide',
    note: 'Detects your system · Free and open source · macOS 12+ / Windows 10+ / Linux (GTK 4)',
  },
  shot: {
    caption:
      'Once connected: cluster health, live throughput, consumer lag, and broker state at a glance.',
  },
  drivers: {
    label: 'Available',
    supported: [
      'RocketMQ 4.x / 5.x',
      'RabbitMQ 3.x / 4.x',
      'Kafka 3.x / 4.x',
      'Pulsar 3.x / 4.x',
      'Redis Stream 6.0+',
      'MQTT 3.1.1 / 5.0',
      'NATS 2.x',
      'ActiveMQ Classic 5.x / 6.x · Artemis 2.x',
      'NSQ 1.x',
    ],
    planned: 'Planned: Amazon SQS · Google Cloud Pub/Sub · Azure Service Bus and more',
  },
  features: {
    title: 'Why MQ Studio',
    lead: 'Every message queue arrives with a console of its own — different interfaces, different vocabulary, and every one of them a service to deploy and keep alive. MQ Studio is one client for all of them.',
    items: [
      {
        title: 'One interface, every broker',
        body: 'Each broker is reached through a driver behind the same interface, so the pages and the workflow stay the same whichever system you are connected to.',
      },
      {
        title: 'Honest about what it connects to',
        body: 'Every connection reports what its endpoint can actually do, and the pages are drawn from that — never an action the broker cannot perform.',
      },
      {
        title: 'Ready to use',
        body: 'Download, connect, work. There is no server component to deploy, secure, or keep alive.',
      },
      {
        title: 'Private by default',
        body: 'Configuration stays on your device and credentials are encrypted at rest; import and export stay under your control.',
      },
      {
        title: 'Cross-platform and bilingual',
        body: 'macOS, Windows, and Linux, with English and Chinese interfaces and a theme that follows the system.',
      },
      {
        title: 'Open source, auditable',
        body: 'Apache-2.0 licensed with the source fully public, and every release ships SHA256 checksums to verify your download.',
      },
    ],
  },
  modules: {
    title: 'From topics to alerts, in one client',
    lead: 'Every driver is taken to the same depth — below is the real interface on a RocketMQ connection.',
    tabs: [
      {
        id: 'connections',
        label: 'Connections',
        title: 'Connection management',
        desc: 'Every cluster in one list; double-click a row to open it in its own tab. Free-text groups and auto-connect included.',
        points: [
          'Fill in only the endpoints and credentials the protocol needs',
          'Credentials encrypted at rest on your machine',
          'Import and export configuration to move between devices',
        ],
        alt: 'The MQ Studio connection list',
      },
      {
        id: 'topics',
        label: 'Topics',
        title: 'Topic operations',
        desc: 'Filter by type, then inspect queues, routing, and subscriptions in the detail panel.',
        points: [
          'Create and inspect topics, queues, exchanges, and bindings',
          'Review partitions, settings, and arguments',
          'Selectors match on fuzzy input and remember what you used',
        ],
        alt: 'The MQ Studio topic list and detail panel',
      },
      {
        id: 'consumers',
        label: 'Consumers',
        title: 'Consumer diagnostics',
        desc: 'Lag, consume TPS, and clients per group; reset or clone offsets queue by queue.',
        points: [
          'View groups, clients, and subscriptions',
          'Reset offsets per queue',
          'Work through retry queues and dead letters',
        ],
        alt: 'The MQ Studio consumer group list and detail panel',
      },
      {
        id: 'cluster',
        label: 'Cluster',
        title: 'Cluster monitoring',
        desc: 'Broker roles, throughput, disk water level, and messages in and out today, refreshed live.',
        points: [
          'Monitor broker and node runtime metrics',
          'Watch throughput and consumer lag trends',
          'Track disk usage and today’s message volume',
        ],
        alt: 'The MQ Studio cluster monitoring page',
      },
      {
        id: 'alerts',
        label: 'Alerts',
        title: 'Alerts',
        desc: 'Active alerts derived from live cluster metrics, with the rules behind them under your control.',
        points: [
          'Rules you define, with adjustable thresholds',
          'Desktop notifications when one fires',
          'Same source as the cluster metrics, no extra collector',
        ],
        alt: 'The MQ Studio alerts page',
      },
    ],
  },
  roadmap: {
    title: 'Roadmap',
    body: 'Drivers land one at a time: each is taken to the depth RocketMQ already has before the next one starts. This is an order, not a schedule.',
    linkLabel: 'Full roadmap',
    stages: [
      { label: 'RocketMQ', done: true },
      { label: 'RabbitMQ', done: true },
      { label: 'Kafka', done: true },
      { label: 'Redis Stream', done: true },
      { label: 'Pulsar', done: true },
      { label: 'MQTT', done: true },
      { label: 'NATS', done: true },
      { label: 'ActiveMQ', done: true },
      { label: 'NSQ', done: true },
      { label: 'More drivers', done: false },
      { label: 'Agent', done: false },
    ],
  },
  changelog: {
    navLabel: 'Changelog',
    title: 'Changelog',
    lead: 'What changed in every release. Versions follow Semantic Versioning and the format follows Keep a Changelog.',
    metaTitle: 'MQ Studio changelog — what changed in every release',
    metaDescription:
      'The full changelog for every MQ Studio release: what was added, what was fixed, and the known limitations, newest first.',
    latest: 'Latest',
    onThisPage: 'Releases',
    inThisRelease: 'In this release',
    pagination: 'Adjacent releases',
    newer: 'Newer release',
    older: 'Older release',
    onGitHub: 'View on GitHub',
  },
  docs: {
    navLabel: 'Docs',
    sectionTitle: 'Documentation',
    titles: {
      install: 'Install guide',
      architecture: 'Architecture',
      roadmap: 'Roadmap',
    },
    untranslated: 'This page has no Chinese translation yet; the English text is shown below.',
    onThisPage: 'On this page',
    editOnGitHub: 'Edit on GitHub',
  },
  community: {
    title: 'Built in the open',
    lead: 'The source is public and the discussion happens on GitHub. If something gets in your way, or a driver you need is missing, open an issue.',
    stars: 'Stars',
    forks: 'Forks',
    contributors: 'Contributors',
    license: 'License',
    builtBy: 'Maintained by',
    ctaIssues: 'Open an issue',
    ctaRepo: 'Browse the source',
  },
  download: {
    title: 'Download MQ Studio',
    leadPrefix: 'Current release',
    leadSuffix: 'every release ships a {checksums}',
    mirror: 'Alternate',
    platforms: {
      mac: { name: 'macOS', requirement: 'macOS 12 and later', cta: 'Download .dmg' },
      windows: { name: 'Windows', requirement: 'Windows 10 and later', cta: 'Download .exe' },
      linux: { name: 'Linux', requirement: 'GTK 4 · WebKitGTK 6.0', cta: 'Download for Linux' },
    },
    archLabels: { amd64: 'Intel / x64', arm64: 'Apple silicon', winAmd64: 'x64', winArm64: 'ARM64' },
    noteMac:
      'The macOS build is not signed with a developer certificate yet, so the first launch takes one extra step; the disk image ships a script for it — see the',
    noteMacLink: 'install guide',
    noteLinux:
      'Linux packages need Ubuntu 24.04+ / Debian 13+ or the equivalent elsewhere · earlier versions are under',
    noteLinuxLink: 'all releases',
    selectArch: 'Select architecture',
    selectFormat: 'Select format',
  },
  footer: {
    tagline: 'A local-first desktop client for message queues.',
    copyright: '© 2026 amigoer · Apache-2.0',
    groups: [
      {
        title: 'Product',
        links: [
          { label: 'Features', href: '#features' },
          { label: 'Product tour', href: '#modules' },
          { label: 'Roadmap', href: '#roadmap' },
          { label: 'Download', href: '#download' },
        ],
      },
      {
        title: 'Resources',
        links: [
          { label: 'Install guide', href: '/en/docs/install/' },
          { label: 'Architecture', href: '/en/docs/architecture/' },
          { label: 'Changelog', href: '/en/changelog/' },
          { label: 'Roadmap', href: '/en/docs/roadmap/' },
        ],
      },
      {
        title: 'Community',
        links: [
          { label: 'GitHub', href: 'https://github.com/amigoer/mq-studio' },
          { label: 'Issues', href: 'https://github.com/amigoer/mq-studio/issues' },
        ],
      },
    ],
  },
};
