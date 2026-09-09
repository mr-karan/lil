import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting'
import tsParser from '@typescript-eslint/parser'

export default [
  {
    name: 'app/files-to-lint',
    files: ['**/*.{js,mjs,jsx,ts,vue}'],
  },

  {
    name: 'app/files-to-ignore',
    ignores: ['**/dist/**', '**/dist-ssr/**', '**/coverage/**'],
  },

  js.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  {
    files: ['**/*.vue'],
    languageOptions: { parserOptions: { parser: tsParser } },
    rules: { 'no-undef': 'off' },
  },
  {
    files: ['**/*.ts'],
    languageOptions: { parser: tsParser },
    rules: { 'no-undef': 'off', 'no-unused-vars': 'off' },
  },
  {
    languageOptions: {
      globals: {
        window: 'readonly', document: 'readonly', navigator: 'readonly',
        localStorage: 'readonly', console: 'readonly', fetch: 'readonly',
        setTimeout: 'readonly', confirm: 'readonly', HTMLDialogElement: 'readonly',
        process: 'readonly',
      },
    },
  },
  skipFormatting,
]
