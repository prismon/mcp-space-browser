import pino from 'pino';

const isTest = process.env.NODE_ENV === 'test' || process.argv.includes('test');

export const logger = pino({
  level: process.env.LOG_LEVEL || (isTest ? 'silent' : 'info'),
  transport: !isTest ? {
    target: 'pino-pretty',
    options: {
      colorize: true,
      translateTime: 'SYS:standard',
      ignore: 'pid,hostname',
    }
  } : undefined,
});

export function createChildLogger(name: string) {
  return logger.child({ name });
}