/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#fafafa',
          100: '#f5f5f5',
          200: '#e5e5e5',
          300: '#d4d4d4',
          400: '#a3a3a3',
          500: '#737373',
          600: '#525252',
          700: '#404040',
          800: '#262626',
          900: '#171717',
          950: '#0a0a0a'
        },
        accent: {
          50: '#faf9f6',
          100: '#f1eee8',
          200: '#e0d8d0',
          300: '#c9a227',
          400: '#8a8a8a',
          500: '#666666',
          600: '#525252',
          700: '#3f3f3f',
          800: '#292929',
          900: '#1f1f1f',
          950: '#111111'
        },
        dark: {
          50: '#f5f5f5',
          100: '#e5e5e5',
          200: '#d4d4d4',
          300: '#a3a3a3',
          400: '#737373',
          500: '#525252',
          600: '#404040',
          700: '#333333',
          800: '#242424',
          900: '#1a1a1a',
          950: '#0f0f0f'
        }
      },
      fontFamily: {
        display: ['Playfair Display', 'Georgia', 'serif'],
        serif: ['Playfair Display', 'Georgia', 'serif'],
        sans: [
          'Inter',
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
        body: [
          'Inter',
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
        soft: '0 4px 20px rgba(0, 0, 0, 0.03)',
        'soft-dark': '0 4px 20px rgba(0, 0, 0, 0.3)',
        card: '0 1px 2px rgba(0, 0, 0, 0.04)',
        'card-hover': '0 8px 24px rgba(0, 0, 0, 0.08)',
        glass: '0 4px 20px rgba(0, 0, 0, 0.04)',
        'glass-sm': '0 2px 12px rgba(0, 0, 0, 0.04)'
      },
      backgroundImage: {
        'gradient-primary': 'linear-gradient(135deg, #222222 0%, #333333 100%)',
        'gradient-dark': 'linear-gradient(135deg, #1a1a1a 0%, #0f0f0f 100%)',
        'mesh-gradient': 'linear-gradient(180deg, rgba(250,250,250,0.9) 0%, rgba(255,255,255,1) 100%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite'
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
