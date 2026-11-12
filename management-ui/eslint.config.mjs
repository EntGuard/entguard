import angular from '@angular-eslint/eslint-plugin';
import angularTemplate from '@angular-eslint/eslint-plugin-template';
import tsParser from '@typescript-eslint/parser';
import angularTemplateParser from '@angular-eslint/template-parser';
import tsPlugin from '@typescript-eslint/eslint-plugin';
import angularEslint from 'angular-eslint';

const rulesOf = (configs) => Object.assign({}, ...configs.map((c) => c.rules ?? {}));

export default [
  {
    ignores: ['projects/**/*', 'node_modules/**', 'dist/**', '.angular/**'],
  },
  // TypeScript files configuration
  {
    files: ['**/*.ts'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: ['./tsconfig.json'],
        createDefaultProgram: true,
        ecmaVersion: 2022,
        sourceType: 'module',
      },
    },
    plugins: {
      '@angular-eslint': angular,
      '@typescript-eslint': tsPlugin,
    },
    rules: {
      ...rulesOf(angularEslint.configs.tsRecommended),
      '@angular-eslint/directive-selector': [
        'error',
        {
          type: 'attribute',
          // 'mat' is allowed for directives that deliberately extend Angular
          // Material behaviour and mirror its attribute naming.
          prefix: ['app', 'mat'],
          style: 'camelCase',
        },
      ],
      '@angular-eslint/component-selector': [
        'error',
        {
          type: 'element',
          prefix: 'app',
          style: 'kebab-case',
        },
      ],
      '@angular-eslint/prefer-standalone': 'off',
      '@angular-eslint/prefer-inject': 'off',
      '@angular-eslint/prefer-on-push-component-change-detection': 'off',
    },
  },
  // HTML template files configuration
  {
    files: ['**/*.html'],
    languageOptions: {
      parser: angularTemplateParser,
    },
    plugins: {
      '@angular-eslint/template': angularTemplate,
    },
    rules: {
      ...rulesOf(angularEslint.configs.templateRecommended),
    },
  },
];
