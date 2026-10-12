// Vite empties the embed folder on build; put the tracked placeholder back so git stays clean.
import { writeFileSync } from 'node:fs'
writeFileSync(new URL('../../internal/adminui/dist/.gitkeep', import.meta.url), '')
