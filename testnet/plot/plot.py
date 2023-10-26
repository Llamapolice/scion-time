# Based on plot.py by Julian, https://github.com/XYQuadrat

import dateutil
import matplotlib.dates
import matplotlib.pyplot
import pandas
import sys

log = pandas.read_csv(sys.argv[1], names=["t", "off", "rtd"])
x = [dateutil.parser.isoparse(x) for x in log["t"]]
y = [float(y) for y in log["off"]]

matplotlib.pyplot.style.use('style.mplstyle')
ax = matplotlib.pyplot.gca()

ax.plot(x, y)

ax.set_ylim(-0.1, 0.1)
ax.set_ylabel(r'Offset (s)')
ax.set_xlabel(r'Time')

ax.xaxis.set_major_locator(matplotlib.dates.HourLocator((0, 3, 6, 9, 12, 15, 18, 21)))
ax.xaxis.set_major_formatter(matplotlib.dates.DateFormatter("%H:%M"))

matplotlib.pyplot.savefig(sys.argv[1] + ".pdf")
