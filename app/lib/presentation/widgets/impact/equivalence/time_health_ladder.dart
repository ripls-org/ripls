import 'equivalence_tier.dart';

/// kTimeHealthLadder is the 16-tier "What it's doing for your body"
/// ladder for the community Time Together metric. Thresholds in hours.
/// Authored verbatim from `docs/cowork/Data Scientist/Ripls_Equivalence_Spec.md`.
const List<EquivalenceTier> kTimeHealthLadder = [
  EquivalenceTier(
    id: 't_5min',
    threshold: 5 / 60,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT5min',
    copyKey: 'equivalenceTimeHealthCopyT5min',
    sourceNameKey: 'equivalenceTimeHealthSourcePsychologyToday',
    sourceUrl:
        'https://www.psychologytoday.com/us/blog/keep-it-in-mind/202201/what-20-seconds-hugging-can-do-you',
  ),
  EquivalenceTier(
    id: 't_1hr',
    threshold: 1,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT1hr',
    copyKey: 'equivalenceTimeHealthCopyT1hr',
    sourceNameKey: 'equivalenceTimeHealthSourceAha',
    sourceUrl:
        'https://www.heart.org/en/news/2024/02/12/how-a-social-connection-expert-stays-connected-and-why',
  ),
  EquivalenceTier(
    id: 't_3hr',
    threshold: 3,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT3hr',
    copyKey: 'equivalenceTimeHealthCopyT3hr',
    sourceNameKey: 'equivalenceTimeHealthSourceAtlanticHealth',
    sourceUrl:
        'https://www.atlantichealth.org/health-articles/mental-wellness/strong-social-connections-boost-your-health-and-longevity',
  ),
  EquivalenceTier(
    id: 't_5hr',
    threshold: 5,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT5hr',
    copyKey: 'equivalenceTimeHealthCopyT5hr',
    sourceNameKey: 'equivalenceTimeHealthSourceElsa',
    sourceUrl: 'https://pmc.ncbi.nlm.nih.gov/articles/PMC7820553/',
  ),
  EquivalenceTier(
    id: 't_10hr',
    threshold: 10,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT10hr',
    copyKey: 'equivalenceTimeHealthCopyT10hr',
    sourceNameKey: 'equivalenceTimeHealthSourcePmcCardio',
    sourceUrl: 'https://pmc.ncbi.nlm.nih.gov/articles/PMC2765114/',
  ),
  EquivalenceTier(
    id: 't_15hr',
    threshold: 15,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT15hr',
    copyKey: 'equivalenceTimeHealthCopyT15hr',
    sourceNameKey: 'equivalenceTimeHealthSourcePmcConnection',
    sourceUrl: 'https://pmc.ncbi.nlm.nih.gov/articles/PMC11403199/',
  ),
  EquivalenceTier(
    id: 't_25hr',
    threshold: 25,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT25hr',
    copyKey: 'equivalenceTimeHealthCopyT25hr',
    sourceNameKey: 'equivalenceTimeHealthSourceHoltLunstad',
    sourceUrl:
        'https://journals.plos.org/plosmedicine/article?id=10.1371%2Fjournal.pmed.1000316',
  ),
  EquivalenceTier(
    id: 't_40hr',
    threshold: 40,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT40hr',
    copyKey: 'equivalenceTimeHealthCopyT40hr',
    sourceNameKey: 'equivalenceTimeHealthSourceUnhExtension',
    sourceUrl:
        'https://extension.unh.edu/blog/2022/05/prolonged-social-isolation-loneliness-are-equivalent-smoking-15-cigarettes-day',
  ),
  EquivalenceTier(
    id: 't_60hr',
    threshold: 60,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT60hr',
    copyKey: 'equivalenceTimeHealthCopyT60hr',
    sourceNameKey: 'equivalenceTimeHealthSourceSurgeonGeneral',
    sourceUrl:
        'https://www.hhs.gov/sites/default/files/surgeon-general-social-connection-advisory.pdf',
  ),
  EquivalenceTier(
    id: 't_100hr',
    threshold: 100,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT100hr',
    copyKey: 'equivalenceTimeHealthCopyT100hr',
    sourceNameKey: 'equivalenceTimeHealthSourceCornellChronicle',
    sourceUrl:
        'https://news.cornell.edu/stories/2025/09/lifetime-social-ties-adds-healthy-aging',
  ),
  EquivalenceTier(
    id: 't_150hr',
    threshold: 150,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT150hr',
    copyKey: 'equivalenceTimeHealthCopyT150hr',
    sourceNameKey: 'equivalenceTimeHealthSourceMahalingam',
    sourceUrl:
        'https://alz-journals.onlinelibrary.wiley.com/doi/10.1002/alz.13072',
  ),
  EquivalenceTier(
    id: 't_250hr',
    threshold: 250,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT250hr',
    copyKey: 'equivalenceTimeHealthCopyT250hr',
    sourceNameKey: 'equivalenceTimeHealthSourceRushUniversity',
    sourceUrl:
        'https://www.rush.edu/news/being-social-may-delay-dementia-onset-five-years',
  ),
  EquivalenceTier(
    id: 't_500hr',
    threshold: 500,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT500hr',
    copyKey: 'equivalenceTimeHealthCopyT500hr',
    sourceNameKey: 'equivalenceTimeHealthSourceScientificAmerican',
    sourceUrl:
        'https://www.scientificamerican.com/article/relationships-boost-survival/',
  ),
  EquivalenceTier(
    id: 't_1000hr',
    threshold: 1000,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT1000hr',
    copyKey: 'equivalenceTimeHealthCopyT1000hr',
    sourceNameKey: 'equivalenceTimeHealthSourceHarvardChan',
    sourceUrl:
        'https://hsph.harvard.edu/news/the-importance-of-connections-ways-to-live-a-longer-healthier-life/',
  ),
  EquivalenceTier(
    id: 't_2000hr',
    threshold: 2000,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelT2000hr',
    copyKey: 'equivalenceTimeHealthCopyT2000hr',
    sourceNameKey: 'equivalenceTimeHealthSourceHarvardChan',
    sourceUrl:
        'https://hsph.harvard.edu/news/the-importance-of-connections-ways-to-live-a-longer-healthier-life/',
  ),
  EquivalenceTier(
    id: 't_lifetime',
    threshold: 5000,
    unit: EquivalenceUnit.hours,
    labelKey: 'equivalenceTimeHealthLabelTLifetime',
    copyKey: 'equivalenceTimeHealthCopyTLifetime',
    sourceNameKey: 'equivalenceTimeHealthSourcePsyPost',
    sourceUrl:
        'https://www.psypost.org/lifelong-social-connections-may-slow-biological-aging-and-reduce-inflammation/',
  ),
];
