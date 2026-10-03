# Bibliography

References used by the KB generators, with where each one is used. Keep
entries in the citation style below and add the "Used by" line so a future
reader knows what depends on what.

## Tectonics (`pkg/tectonics`, `cmd/tectonics-lab`)

### Plate reconstructions (Earth reference data)

- **Merdith, A. S., Williams, S. E., Collins, A. S., Tetley, M. G., Mulder, J. A., Blades, M. L., Young, A., Armistead, S. E., Cannon, J., Zahirovic, S., Müller, R. D. (2021).** Extending full-plate tectonic models into deep time: Linking the Neoproterozoic and the Phanerozoic. *Earth-Science Reviews*, 214, 103477. https://doi.org/10.1016/j.earscirev.2020.103477
  Data: plate model (rotations + topologies, 1000 Ma to present), Zenodo, CC-BY 4.0. https://zenodo.org/records/4485738
  Used by: the "Earth" reference bundle (`scripts/earth_plates_export.py`), level-2 statistics benchmark.
- **Müller, R. D., Flament, N., Cannon, J., Tetley, M. G., Williams, S. E., Cao, X., Bodur, Ö. F., Zahirovic, S., Merdith, A. (2022).** A tectonic-rules-based mantle reference frame since 1 billion years ago: implications for supercontinent cycles and plate–mantle system evolution. *Solid Earth*, 13, 1127–1159. https://doi.org/10.5194/se-13-1127-2022
  Data: the Merdith et al. (2021) model with optimised-mantle and no-net-rotation reference frames, v1.1. https://www.earthbyte.org/webdav/ftp/Data_Collections/Muller_etal_2022_SE/Muller_etal_2022_SE_1Ga_Opt_PlateMotionModel_v1.1.zip
  Used by: the Earth reference bundle (no-net-rotation frame for speed comparisons).
- **Bird, P. (2003).** An updated digital model of plate boundaries. *Geochemistry, Geophysics, Geosystems*, 4(3), 1027. https://doi.org/10.1029/2001GC000252
  Data: PB2002 plate boundaries, plates, orogens and poles. http://peterbird.name/oldFTP/PB2002/
  Used by: present-day plate statistics (count, sizes, boundary classes).
- **DeMets, C., Gordon, R. G., Argus, D. F. (2010).** Geologically current plate motions. *Geophysical Journal International*, 181(1), 1–80. https://doi.org/10.1111/j.1365-246X.2009.04491.x
  Data: MORVEL angular velocities for 25 plates; online calculator. http://www.geology.wisc.edu/~chuck/MORVEL/
  Used by: present-day speed distribution.

Component models the Merdith et al. (2021) compilation is built from (cited in its README):

- **Domeier, M. (2016).** A plate tectonic scenario for the Iapetus and Rheic oceans. *Gondwana Research*, 36, 275–295.
- **Domeier, M. (2018).** Early Paleozoic tectonics of Asia: towards a full-plate model. *Geoscience Frontiers*, 9(3), 789–862.
- **Matthews, K. J., Maloney, K. T., Zahirovic, S., Williams, S. E., Seton, M., Müller, R. D. (2016).** Global plate boundary evolution and kinematics since the late Paleozoic. *Global and Planetary Change*, 146, 226–250.
- **Merdith, A. S., Collins, A. S., Williams, S. E., Pisarevsky, S., Foden, J. D., Archibald, D. B., Blades, M. L., Alessio, B. L., Armistead, S., Plavsa, D., Clark, C. (2017).** A full-plate global reconstruction of the Neoproterozoic. *Gondwana Research*, 50, 84–134.
- **Torsvik, T. H., Steinberger, B., Shephard, G. E., Doubrovine, P. V., Gaina, C., Domeier, M., Conrad, C. P., Sager, W. W. (2019).** Pacific-Panthalassic reconstructions: overview, errata and the way forward. *Geochemistry, Geophysics, Geosystems*, 20(7), 3659–3689.
- **Young, A., Flament, N., Maloney, K., Williams, S., Matthews, K., Zahirovic, S., Müller, R. D. (2019).** Global kinematics of tectonic plates and subduction zones since the late Paleozoic Era. *Geoscience Frontiers*, 10(3), 989–1013.

### Software

- **GPlates / pyGPlates.** Müller, R. D., Cannon, J., Qin, X., Watson, R. J., Gurnis, M., Williams, S., Pfaffelmoser, T., Seton, M., Russell, S. H. J., Zahirovic, S. (2018). GPlates: Building a virtual Earth through deep time. *Geochemistry, Geophysics, Geosystems*, 19(7), 2243–2261. https://doi.org/10.1029/2018GC007584
  pyGPlates 1.0 documentation: https://www.gplates.org/docs/pygplates/
  Used by: `scripts/earth_plates_export.py` (resolving plate topologies and velocities per time step).
- **GPlates Web Service.** https://gwsdoc.gplates.org/ — HTTP API over the same models; used only for spot checks.

### Background on the simulation approach

- Wilson-cycle / supercontinent framing of the drift rules follows the plate-boundary conceptualisation in the Merdith et al. (2021) README ("plate motion moves orthogonally from a mid-ocean ridge towards a subduction zone, connected by transforms").
