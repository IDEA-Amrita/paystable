import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import mermaid from 'astro-mermaid';

// https://astro.build/config
export default defineConfig({
	integrations: [
		mermaid(),
		starlight({
			title: 'Paystable Docs',
			customCss: [
				'./src/styles/custom.css',
			],
			social: {
				github: 'https://github.com/IDEA-Amrita/paystable',
			},
			sidebar: [
				{
					label: 'Guides',
					items: [
						{ label: 'Getting Started', link: '/guides/getting-started/' },
						{ label: 'Frontend UX', link: '/guides/frontend-ux/' },
					],
				},
				{
					label: 'Technical Reference',
					items: [
						{ label: 'API Specification', link: '/reference/api/' },
						{ label: 'Database Schema', link: '/reference/schema/' },
						{ label: 'Callback Contract', link: '/reference/callbacks/' },
					],
				},
				{
					label: 'Project Readiness',
					items: [
						{ label: 'Gap Analysis', link: '/reference/gap-analysis/' },
					],
				},
			],
		}),
	],
});
