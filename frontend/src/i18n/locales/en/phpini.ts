import type { phpini as base } from '../pt-BR/phpini'

export const phpini: typeof base = {
  toggle: 'php.ini',
  label: 'PHP.INI',
  hint: 'Saving restarts the PHP {major} workers. Directives managed by HyPHP (paths, mail, cgi.*) are refused.',
  sourceHyphp: 'HyPHP',
  sourcePhp: 'PHP',
  sourceUser: 'custom',
  defaultIs: 'default: {value}',
  emptyValue: '(empty)',
  valueAria: 'Value of {name}',
  save: 'Save',
  reset: 'Restore default',
  addLabel: 'Add directive',
  namePlaceholder: 'max_input_vars',
  valuePlaceholder: '5000',
  newNameAria: 'New directive name',
  newValueAria: 'New directive value',
  add: 'Add',
}
