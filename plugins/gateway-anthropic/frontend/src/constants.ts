/** AWS Bedrock region options */
export const BEDROCK_REGION_OPTIONS = [
  {
    label: 'US',
    options: [
      { value: 'us-east-1', label: 'us-east-1 (N. Virginia)' },
      { value: 'us-east-2', label: 'us-east-2 (Ohio)' },
      { value: 'us-west-1', label: 'us-west-1 (N. California)' },
      { value: 'us-west-2', label: 'us-west-2 (Oregon)' },
      { value: 'us-gov-east-1', label: 'us-gov-east-1 (GovCloud US-East)' },
      { value: 'us-gov-west-1', label: 'us-gov-west-1 (GovCloud US-West)' },
    ],
  },
  {
    label: 'Europe',
    options: [
      { value: 'eu-west-1', label: 'eu-west-1 (Ireland)' },
      { value: 'eu-west-2', label: 'eu-west-2 (London)' },
      { value: 'eu-west-3', label: 'eu-west-3 (Paris)' },
      { value: 'eu-central-1', label: 'eu-central-1 (Frankfurt)' },
      { value: 'eu-central-2', label: 'eu-central-2 (Zurich)' },
      { value: 'eu-south-1', label: 'eu-south-1 (Milan)' },
      { value: 'eu-south-2', label: 'eu-south-2 (Spain)' },
      { value: 'eu-north-1', label: 'eu-north-1 (Stockholm)' },
    ],
  },
  {
    label: 'Asia Pacific',
    options: [
      { value: 'ap-northeast-1', label: 'ap-northeast-1 (Tokyo)' },
      { value: 'ap-northeast-2', label: 'ap-northeast-2 (Seoul)' },
      { value: 'ap-northeast-3', label: 'ap-northeast-3 (Osaka)' },
      { value: 'ap-south-1', label: 'ap-south-1 (Mumbai)' },
      { value: 'ap-south-2', label: 'ap-south-2 (Hyderabad)' },
      { value: 'ap-southeast-1', label: 'ap-southeast-1 (Singapore)' },
      { value: 'ap-southeast-2', label: 'ap-southeast-2 (Sydney)' },
    ],
  },
  { label: 'Canada', options: [{ value: 'ca-central-1', label: 'ca-central-1 (Canada)' }] },
  { label: 'South America', options: [{ value: 'sa-east-1', label: 'sa-east-1 (Sao Paulo)' }] },
] as const
