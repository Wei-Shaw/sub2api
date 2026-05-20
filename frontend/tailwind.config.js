/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - Warm Gold 暖金色系 (Build style)
        primary: {
          50: '#fdf8f0',
          100: '#f9edda',
          200: '#f2d9b0',
          300: '#e9bf7e',
          400: '#D4A574',
          500: '#c8914f',
          600: '#b47a3a',
          700: '#956232',
          800: '#7a4f2c',
          900: '#654228',
          950: '#372013'
        },
        // 辅助色 - Warm Neutral 暖灰
        accent: {
          50: '#FAFAF8',
          100: '#f5f3ef',
          200: '#e8e4dd',
          300: '#d5cfc4',
          400: '#b8b0a2',
          500: '#9c9486',
          600: '#837a6d',
          700: '#6a6259',
          800: '#1a1a1a',
          900: '#111111',
          950: '#0a0a0a'
        },
        // 页面背景色
        surface: '#FAFAF8',
        // 深色模式背景（暖灰色调，与 起源AI 暖金品牌色搭配）
        dark: {
          50: '#fafaf9',
          100: '#f5f5f4',
          200: '#e7e5e4',
          300: '#d6d3d1',
          400: '#a8a29e',
          500: '#78716c',
          600: '#57534e',
          700: '#44403c',
          800: '#292524',
          900: '#1c1917',
          950: '#0c0a09'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 1px 3px rgba(0, 0, 0, 0.06)',
        'glass-sm': '0 1px 2px rgba(0, 0, 0, 0.04)',
        glow: '0 0 20px rgba(212, 165, 116, 0.2)',
        'glow-lg': '0 0 30px rgba(212, 165, 116, 0.3)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04)',
        'card-hover': '0 4px 12px rgba(0, 0, 0, 0.06)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #D4A574 0%, #c8914f 100%)',
        'gradient-dark': 'linear-gradient(135deg, #1a1a1a 0%, #111111 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(212, 165, 116, 0.08) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(200, 145, 79, 0.06) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(212, 165, 116, 0.04) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(212, 165, 116, 0.2)' },
          '100%': { boxShadow: '0 0 30px rgba(212, 165, 116, 0.3)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
